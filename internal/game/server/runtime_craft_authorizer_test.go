package server

import (
	"reflect"
	"testing"
)

func TestRuntimeWiresCraftLocationAuthorizer(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	craftingValue := reflect.ValueOf(gameServer.runtime.Crafting).Elem()
	locationAuth := craftingValue.FieldByName("locationAuth")
	if !locationAuth.IsValid() {
		t.Fatal("CraftingService.locationAuth field missing")
	}
	if locationAuth.IsNil() {
		t.Fatal("runtime CraftingService location authorizer is nil")
	}
}
