package main

import (
	"os"
	"testing"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
)

func TestRunsSuite(t *testing.T) {
	fixture := acmetest.NewFixture(&betterWapiDNSProviderSolver{},
		acmetest.SetResolvedZone(zone),
		acmetest.SetManifestPath("testdata/better-wapi"),
	)

	fixture.RunBasic(t)
	fixture.RunExtended(t)
}
