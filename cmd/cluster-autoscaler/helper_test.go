package clusterautoscaler

import (
	"github.com/mumoshu/testkit"
	"testing"
)

func helmInstallKwok(t *testing.T, helm *testkit.Helm) {
	helm.AddRepo(t, "kwok", "https://kwok.sigs.k8s.io/charts")

	helm.UpgradeOrInstall(t, "kwok", "kwok/kwok", func(hc *testkit.HelmConfig) {
		hc.Namespace = "kube-system"
	})
	// why: install stage rules to simulate Pod/Node lifecycle in a KWOK simulated cluster
	helm.UpgradeOrInstall(t, "kwok-stage-fast", "kwok/stage-fast", func(hc *testkit.HelmConfig) {
		hc.Namespace = "kube-system"
	})
	t.Logf("KWOK installed successfully")
}
