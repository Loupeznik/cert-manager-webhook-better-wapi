package main

import (
	"os"
	"testing"

	"github.com/cert-manager/cert-manager/test/acme"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
)

func TestRunsSuite(t *testing.T) {
	fixture := acme.NewFixture(&betterWapiDNSProviderSolver{},
		acme.SetResolvedZone(zone),
		acme.SetAllowAmbientCredentials(false),
		acme.SetManifestPath("testdata/better-wapi"),
	)

	fixture.RunConformance(t)
}
