[![Go](https://github.com/BulletTrainHQ/bullet-train-go-client/workflows/Go/badge.svg)](https://github.com/BulletTrainHQ/bullet-train-go-client/actions)
[![GoReportCard](https://goreportcard.com/badge/github.com/BulletTrainHQ/bullet-train-go-client)](https://goreportcard.com/report/github.com/BulletTrainHQ/bullet-train-go-client)
[![GoDoc](https://godoc.org/github.com/BulletTrainHQ/bullet-train-go-client?status.svg)](https://godoc.org/github.com/BulletTrainHQ/bullet-train-go-client)

<img width="100%" src="https://raw.githubusercontent.com/SolidStateGroup/bullet-train-frontend/master/hero.png"/>

# Bullet Train SDK for Go

Bullet Train allows you to manage feature flags and remote config across multiple projects, environments and organisations.

This is the SDK for go for [https://bullet-train.io/](https://bullet-train.io/).

## Getting Started

```bash
go get github.com/BulletTrainHQ/bullet-train-go-client
```

```go
import (
  bullettrain "github.com/BulletTrainHQ/bullet-train-go-client"
)
```

## Usage

### Retrieving feature flags for your project

For full documentation visit [https://docs.bullet-train.io](https://docs.bullet-train.io)

Sign Up and create account at [https://bullet-train.io/](https://www.bullet-train.io/)

In your application initialise the BulletTrain client with your API key

```go
bt := bullettrain.DefaultClient("<Your API Key>")
```

To check if a feature flag exists and is enabled:

```go
bt := bullettrain.DefaultClient("<Your API Key>")
enabled, err := bt.FeatureEnabled("cart_abundant_notification_ab_test_enabled")
if err != nil {
    log.Fatal(err)
} else {
    if (enabled) {
        fmt.Printf("Feature enabled")
    }
}
```

To get the configuration value for feature flag value:

```go
featureValue, err := bt.GetValue("cart_abundant_notification_ab_test")
if err != nil {
    log.Fatal(err)
} else {
    fmt.Println(featureValue)
}
```

More examples can be found in the [Tests](client_test.go)

## Override default configuration

By default, client is using default configuration. You can override configuration as follows:

```go
bt := bullettrain.NewClient("<Your API Key>", bullettrain.Config{BaseURI: "<Your API URL>"})
```

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/kyle-ssg/c36a03aebe492e45cbd3eefb21cb0486) for details on our code of conduct, and the process for submitting pull requests to us.

## Getting Help

If you encounter a bug or feature request we would like to hear about it. Before you submit an issue please search existing issues in order to prevent duplicates.

## Get in touch

If you have any questions about our projects you can email <a href="mailto:support@bullet-train.io">support@bullet-train.io</a>.
