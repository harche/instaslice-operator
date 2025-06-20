package scheduler

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/utils/clock"

	sched "github.com/openshift/instaslice-operator/pkg/scheduler"
	"github.com/openshift/instaslice-operator/pkg/version"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
)

// NewScheduler creates a command for running the scheduler with controller defaults.
func NewScheduler(ctx context.Context) *cobra.Command {
	cfg := controllercmd.NewControllerCommandConfig("das-scheduler", version.Get(), sched.RunScheduler, clock.RealClock{})
	cfg.DisableLeaderElection = true

	cmd := cfg.NewCommandWithContext(ctx)
	cmd.Use = "scheduler"
	cmd.Short = "Instaslice scheduler"

	return cmd
}
