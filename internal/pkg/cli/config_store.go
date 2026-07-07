package cli

import (
	"context"

	"github.com/aproint/copilot-cli/internal/pkg/aws/identity"
	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

func newSSMConfigStore(sess *session.Session) (*config.Store, error) {
	return newSSMConfigStoreFromConfig(v2ConfigFromSessionRegion(sess)), nil
}

func newSSMConfigStoreFromConfig(cfg aws.Config) *config.Store {
	return config.NewSSMStore(identity.New(cfg), config.NewSSMClient(cfg), cfg.Region)
}

func v2ConfigFromSessionRegion(sess *session.Session) aws.Config {
	cfg, _ := sessions.ImmutableProvider().DefaultConfigWithRegion(context.Background(), aws.ToString(sess.Config.Region))
	return cfg
}
