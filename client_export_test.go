package flagsmith

import "context"

func PollEnvironment(client *Client, ctx context.Context) {
    client.pollEnvironment(ctx)
}
