package perplexity

import (
	"testing"
)

// domain_test.go tests the pure, network-free parts of the domain: Info fields
// and the static model list. The HTTP behaviour is covered in perplexity_test.go
// (external test package, uses httptest).

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "perplexity" {
		t.Errorf("Scheme = %q, want perplexity", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "pplx" {
		t.Errorf("Identity.Binary = %q, want pplx", info.Identity.Binary)
	}
}

func TestKnownModels(t *testing.T) {
	if len(KnownModels) == 0 {
		t.Fatal("KnownModels is empty")
	}
	for _, m := range KnownModels {
		if m.Name == "" {
			t.Error("model has empty Name")
		}
		if m.Description == "" {
			t.Error("model has empty Description")
		}
		if m.Context == 0 {
			t.Errorf("model %q has zero Context", m.Name)
		}
	}
}

func TestValidModel(t *testing.T) {
	if !ValidModel("sonar") {
		t.Error("ValidModel(sonar) should be true")
	}
	if !ValidModel("sonar-pro") {
		t.Error("ValidModel(sonar-pro) should be true")
	}
	if ValidModel("gpt-4") {
		t.Error("ValidModel(gpt-4) should be false")
	}
	if ValidModel("") {
		t.Error("ValidModel('') should be false")
	}
}
