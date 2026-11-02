package benchmarks

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v5"
)

const (
	identityOverridesIdentifier = "identity-override-1"
	identifier                  = "user-1"
)

var (
	environments = []string{"small", "medium", "large"}
	traits       = []*flagsmith.Trait{
		{TraitKey: "plan", TraitValue: "plan-7"},
		{TraitKey: "age", TraitValue: 30},
		{TraitKey: "country", TraitValue: "GB"},
		{TraitKey: "version", TraitValue: "2.5.1"},
		{TraitKey: "email", TraitValue: "jane@example.com"},
		{TraitKey: "user_id", TraitValue: 12},
		{TraitKey: "beta", TraitValue: true},
		{TraitKey: "score", TraitValue: 95},
		{TraitKey: "tier", TraitValue: "pro"},
		{TraitKey: "device", TraitValue: "iPhone 15"},
	}
)

type environmentDocumentTransport []byte

func (environmentDocument environmentDocumentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(environmentDocument)),
		ContentLength: int64(len(environmentDocument)),
		Request:       request,
	}, nil
}

// runPerEnvironment runs a benchmark against a locally evaluating client per environment document size.
func runPerEnvironment(b *testing.B, benchmark func(b *testing.B, client *flagsmith.Client)) {
	for _, environment := range environments {
		b.Run(environment, func(b *testing.B) {
			environmentDocument, err := os.ReadFile(filepath.Join(os.Getenv("SDK_BENCHMARKS_ENVIRONMENTS"), environment+".json"))
			if err != nil {
				b.Fatal(err)
			}
			ctx, closeClient := context.WithCancel(context.Background())
			b.Cleanup(closeClient)
			client := flagsmith.NewClient("ser.benchmark",
				flagsmith.WithBaseURL("http://flagsmith.benchmark/api/v1/"),
				flagsmith.WithHTTPClient(&http.Client{Transport: environmentDocumentTransport(environmentDocument)}),
				flagsmith.WithLocalEvaluation(ctx),
				flagsmith.WithEnvironmentRefreshInterval(24*time.Hour),
				flagsmith.WithSlogLogger(slog.New(slog.DiscardHandler)),
			)
			if err := client.UpdateEnvironment(ctx); err != nil {
				b.Fatal(err)
			}
			benchmark(b, client)
		})
	}
}

// BenchmarkGetEnvironmentFlags gets a feature value from the environment flags.
func BenchmarkGetEnvironmentFlags(b *testing.B) {
	runPerEnvironment(b, func(b *testing.B, client *flagsmith.Client) {
		for b.Loop() {
			flags, err := client.GetEnvironmentFlags(context.Background())
			if err != nil {
				b.Fatal(err)
			}
			if _, err := flags.GetFeatureValue("feature_1"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGetIdentityFlagsWithIdentityOverrides checks a feature for an identity with identity overrides.
func BenchmarkGetIdentityFlagsWithIdentityOverrides(b *testing.B) {
	runPerEnvironment(b, func(b *testing.B, client *flagsmith.Client) {
		for b.Loop() {
			flags, err := client.GetIdentityFlags(context.Background(), identityOverridesIdentifier, nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := flags.IsFeatureEnabled("feature_8"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGetIdentityFlagsNoTraits gets a feature value for an identity without traits.
func BenchmarkGetIdentityFlagsNoTraits(b *testing.B) {
	runPerEnvironment(b, func(b *testing.B, client *flagsmith.Client) {
		for b.Loop() {
			flags, err := client.GetIdentityFlags(context.Background(), identifier, nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := flags.GetFeatureValue("feature_1"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGetIdentityFlagsWithTraits gets a feature value for an identity with traits.
func BenchmarkGetIdentityFlagsWithTraits(b *testing.B) {
	runPerEnvironment(b, func(b *testing.B, client *flagsmith.Client) {
		for b.Loop() {
			flags, err := client.GetIdentityFlags(context.Background(), identifier, traits)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := flags.GetFeatureValue("feature_1"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGetIdentitySegmentsWithTraits lists the segments of an identity with traits.
func BenchmarkGetIdentitySegmentsWithTraits(b *testing.B) {
	runPerEnvironment(b, func(b *testing.B, client *flagsmith.Client) {
		for b.Loop() {
			if _, err := client.GetIdentitySegments(identifier, traits); err != nil {
				b.Fatal(err)
			}
		}
	})
}
