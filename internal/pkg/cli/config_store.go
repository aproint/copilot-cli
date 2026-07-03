package cli

import (
	"context"
	"fmt"

	"github.com/aproint/copilot-cli/internal/pkg/aws/identity"
	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

func newSSMConfigStore(sess *session.Session) (*config.Store, error) {
	cfg, err := sessions.ImmutableProvider().DefaultConfigWithRegion(context.Background(), aws.StringValue(sess.Config.Region))
	if err != nil {
		return nil, fmt.Errorf("create default config: %w", err)
	}
	return config.NewSSMStore(identity.New(cfg), ssm.New(sess), aws.StringValue(sess.Config.Region)), nil
}

func v2ConfigFromSessionRegion(sess *session.Session) awsv2.Config {
	cfg, _ := sessions.ImmutableProvider().DefaultConfigWithRegion(context.Background(), aws.StringValue(sess.Config.Region))
	return cfg
}
