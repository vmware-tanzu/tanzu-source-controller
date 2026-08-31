/*
Copyright 2021-2022 VMware, Inc.
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"reconciler.io/runtime/reconcilers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
	"github.com/vmware-tanzu/tanzu-source-controller/controllers"
	"github.com/vmware-tanzu/tanzu-source-controller/server"
	//+kubebuilder:scaffold:imports
)

var (
	scheme     = runtime.NewScheme()
	setupLog   = ctrl.Log.WithName("setup")
	syncPeriod = 10 * time.Hour
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// validateMaxArtifactSize rejects negative quantities. Comparing with CmpInt64
// rather than Value() matters at the extreme: resource.MustParse("10E").Value()
// overflows int64 and returns 0, while CmpInt64(0) correctly reports it as
// positive.
func validateMaxArtifactSize(q resource.Quantity) error {
	if q.CmpInt64(0) < 0 {
		return fmt.Errorf("--maven-artifact-max-size must not be negative, got %q", q.String())
	}
	if q.CmpInt64(0) > 0 && q.Value() == 0 {
		return fmt.Errorf("--maven-artifact-max-size is too large and overflows int64, got %q", q.String())
	}
	return nil
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var artifactAddr string
	var artifactRootDir string
	var artifactHost string
	var caCertPath string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":0", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&artifactAddr, "artifact-bind-address", ":8082", "The address the artifact server binds to.")
	flag.StringVar(&artifactRootDir, "artifact-root-directory", "./artifact-root", "The directory to stash and serve artifacts from.")
	flag.StringVar(&artifactHost, "artifact-host", "localhost:8082", "The host name to use when constructing artifact urls.")
	flag.StringVar(&caCertPath, "ca-cert-path", "", "The path to addition CA certificates.")
	// Default kept in sync with dist/schema.yaml's maven_artifact_max_size.
	maxArtifactSize := resource.QuantityValue{Quantity: resource.MustParse("500Mi")}
	flag.Var(&maxArtifactSize, "maven-artifact-max-size", "Maximum size of a MavenArtifact's downloaded file, and separately of its extracted contents; peak temporary disk usage can reach roughly twice this value. Applies only to MavenArtifact resources, not ImageRepository. Set to 0 to disable the limit.")
	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.RFC3339NanoTimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if err := validateMaxArtifactSize(maxArtifactSize.Quantity); err != nil {
		setupLog.Error(err, "invalid --maven-artifact-max-size")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "db9e3205.apps.tanzu.vmware.com",
		WebhookServer: &webhook.DefaultServer{
			Options: webhook.Options{
				Port: 9443,
			},
		},
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				// wokeignore:rule=disable
				DisableFor: []client.Object{
					&corev1.Secret{},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	certs := []controllers.Cert{{Path: caCertPath}}

	if err = controllers.ImageRepositoryReconciler(
		reconcilers.NewConfig(mgr, &sourcev1alpha1.ImageRepository{}, syncPeriod),
		artifactRootDir, artifactHost, metav1.Now, certs,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ImageRepository")
		os.Exit(1)
	}

	if err = controllers.MavenArtifactReconciler(
		reconcilers.NewConfig(mgr, &sourcev1alpha1.MavenArtifact{}, syncPeriod),
		artifactRootDir,
		artifactHost,
		metav1.Now,
		certs,
		maxArtifactSize.Value(),
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MavenArtifact")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr, &sourcev1alpha1.MavenArtifact{}).
		WithDefaulter(&sourcev1alpha1.MavenArtifactDefaulter{}).
		WithValidator(&sourcev1alpha1.MavenArtifactValidator{}).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MavenArtifact")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// http blob server for artifacts
	mgr.Add(server.New(artifactAddr, artifactRootDir))

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
