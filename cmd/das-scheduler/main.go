package main

import (
	"context"
	"fmt"
	"os"

	schedcmd "github.com/openshift/instaslice-operator/pkg/cmd/scheduler"
)

func main() {
	cmd := schedcmd.NewScheduler(context.Background())
	if err := cmd.Execute(); err != nil {
		_, err2 := fmt.Fprintf(os.Stderr, "%v\n", err)
		if err2 != nil {
			fmt.Printf("Unable to print err to stderr: %v", err2)
		}
		os.Exit(1)
	}
}
