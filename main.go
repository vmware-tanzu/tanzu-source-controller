/*
Copyright 2021-2022 VMware, Inc.
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

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
	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.RFC3339NanoTimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "db9e3205.apps.tanzu.vmware.com",
		SyncPeriod:             &syncPeriod,
		// wokeignore:rule=disable
		ClientDisableCacheFor: []client.Object{
			&corev1.Secret{},
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
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MavenArtifact")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).For(&sourcev1alpha1.MavenArtifact{}).Complete(); err != nil {
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
