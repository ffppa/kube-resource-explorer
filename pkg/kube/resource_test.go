package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMemoryResource(t *testing.T) {
	r := resource.NewQuantity(2*1024*1024, resource.BinarySI)
	m := NewMemoryResource(2 * 1024 * 1024)

	if r.Value() != m.Value() {
		t.Errorf("Expected %v, got %v", r.Value(), m.Value())
	}

	t.Logf("MemoryResource Quantity: %v", m.ToQuantity())

	if r.Value() != m.ToQuantity().Value() {
		t.Errorf("Expected %v, got %v", r.Value(), m.ToQuantity().Value())
	}
}
