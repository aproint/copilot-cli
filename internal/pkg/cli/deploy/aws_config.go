package deploy

import (
	"context"

	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsv1 "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

func v2ConfigFromSessionRegion(sess *session.Session) aws.Config {
	cfg, _ := sessions.ImmutableProvider().DefaultConfigWithRegion(context.Background(), awsv1.StringValue(sess.Config.Region))
	return cfg
}
