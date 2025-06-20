package scheduler

import (
	"context"
	"fmt"
	"time"

	"k8s.io/component-base/cli"
	schedapp "k8s.io/kubernetes/cmd/kube-scheduler/app"
	_ "sigs.k8s.io/scheduler-plugins/apis/config/scheme"

	instaclient "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned"
	instainformers "github.com/openshift/instaslice-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/instaslice-operator/pkg/operator/operatorclient"
	mig "github.com/openshift/instaslice-operator/pkg/scheduler/plugins/mig"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/loglevel"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// RunScheduler starts the scheduler and the log level controller.
func RunScheduler(ctx context.Context, cc *controllercmd.ControllerContext) error {
	klog.InfoS("RunScheduler started")

	cfg := rest.CopyConfig(cc.KubeConfig)
	cfg.AcceptContentTypes = "application/json"
	cfg.ContentType = "application/json"

	opClientset, err := instaclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create instaslice operator client: %w", err)
	}

	operatorNamespace := cc.OperatorNamespace
	if operatorNamespace == "openshift-config-managed" {
		operatorNamespace = "das-operator"
	}

	opInformerFactory := instainformers.NewSharedInformerFactory(opClientset, 10*time.Minute)
	opClient := &operatorclient.DASOperatorSetClient{
		Ctx:               ctx,
		SharedInformer:    opInformerFactory.OpenShiftOperator().V1alpha1().DASOperators().Informer(),
		Lister:            opInformerFactory.OpenShiftOperator().V1alpha1().DASOperators().Lister(),
		OperatorClient:    opClientset.OpenShiftOperatorV1alpha1(),
		OperatorNamespace: operatorNamespace,
	}
	opInformerFactory.Start(ctx.Done())
	klog.InfoS("Starting log level controller", "namespace", operatorNamespace)
	go loglevel.NewClusterOperatorLoggingController(opClient, cc.EventRecorder).Run(ctx, 1)

	command := schedapp.NewSchedulerCommand(
		schedapp.WithPlugin(mig.Name, mig.New),
	)
	code := cli.Run(command)
	if code != 0 {
		return fmt.Errorf("scheduler exited with code %d", code)
	}
	<-ctx.Done()
	klog.InfoS("RunScheduler completed, shutting down")
	return nil
}
