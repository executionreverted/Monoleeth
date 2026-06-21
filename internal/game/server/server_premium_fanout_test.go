package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/premium"
)

func TestPremiumClaimPassiveFanoutNotifiesOwnerSessionsOnly(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	ownerEmail := "premium-owner@example.com"
	ownerCookie := registerPilotWithIdentity(t, httpServer, ownerEmail, "PremiumOwner")
	ownerSecondCookie := loginPilot(t, httpServer, ownerEmail, "correct-password")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "premium-viewer@example.com", "PremiumViewer")

	ownerConn := dialWebSocket(t, httpServer, ownerCookie)
	defer ownerConn.CloseNow()
	ownerSecondConn := dialWebSocket(t, httpServer, ownerSecondCookie)
	defer ownerSecondConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	readBootstrapEvents(t, ownerConn)
	readBootstrapEvents(t, ownerSecondConn)
	readBootstrapEvents(t, viewerConn)

	writeText(t, ownerConn, `{"request_id":"request-premium-owner-entitlements","op":"premium.entitlements","payload":{},"client_seq":1,"v":1}`)
	entitlementsResponse := readResponse(t, ownerConn)
	if !entitlementsResponse.OK {
		t.Fatalf("premium entitlements response = %+v, want success", entitlementsResponse)
	}
	var entitlementsPayload struct {
		Premium premiumSummaryPayload `json:"premium"`
	}
	if err := json.Unmarshal(entitlementsResponse.Payload, &entitlementsPayload); err != nil {
		t.Fatalf("decode premium entitlements: %v", err)
	}
	if len(entitlementsPayload.Premium.Entitlements) != 1 {
		t.Fatalf("premium entitlements = %+v, want one owner entitlement", entitlementsPayload.Premium.Entitlements)
	}
	entitlementID := entitlementsPayload.Premium.Entitlements[0].EntitlementID

	claimRequest := `{"request_id":"request-premium-passive-claim","op":"premium.claim","payload":{"entitlement_id":"` + entitlementID + `"},"client_seq":2,"v":1}`
	writeText(t, ownerConn, claimRequest)
	claimResponse := readResponse(t, ownerConn)
	if !claimResponse.OK {
		t.Fatalf("premium claim response = %+v, want success", claimResponse)
	}
	var claimPayload premiumMutationPayload
	if err := json.Unmarshal(claimResponse.Payload, &claimPayload); err != nil {
		t.Fatalf("decode premium claim: %v", err)
	}
	if claimPayload.Wallet.PremiumEarned != 50 || claimPayload.Premium.Entitlements[0].State != premium.EntitlementStateClaimed.String() {
		t.Fatalf("premium claim payload = %+v, want claimed entitlement and earned premium wallet", claimPayload)
	}

	ownerClaim := assertPremiumClaimedEvent(t, "owner claim", readEvent(t, ownerConn), entitlementID)
	ownerWallet := assertWalletSnapshotEvent(t, "owner claim wallet", readEvent(t, ownerConn))
	if ownerWallet.PremiumEarned != 50 {
		t.Fatalf("owner claim wallet = %+v, want earned premium 50", ownerWallet)
	}
	ownerSecondClaim := assertPremiumClaimedEvent(t, "second owner claim", readEvent(t, ownerSecondConn), entitlementID)
	ownerSecondWallet := assertWalletSnapshotEvent(t, "second owner claim wallet", readEvent(t, ownerSecondConn))
	if ownerSecondWallet.PremiumEarned != 50 {
		t.Fatalf("second owner claim wallet = %+v, want earned premium 50", ownerSecondWallet)
	}
	if ownerClaim.State != premium.EntitlementStateClaimed.String() || ownerSecondClaim.State != premium.EntitlementStateClaimed.String() {
		t.Fatalf("claim fanout states = %s/%s, want claimed", ownerClaim.State, ownerSecondClaim.State)
	}
	assertNoRealtimeMessageWithin(t, "unrelated premium claim fanout", viewerConn, 100*time.Millisecond)

	duplicateViewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer duplicateViewerConn.CloseNow()
	readBootstrapEvents(t, duplicateViewerConn)

	writeText(t, ownerConn, claimRequest)
	duplicateClaimResponse := readResponse(t, ownerConn)
	if !duplicateClaimResponse.OK {
		t.Fatalf("duplicate premium claim response = %+v, want cached success", duplicateClaimResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate claim owner fanout", ownerConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate claim second owner fanout", ownerSecondConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate claim unrelated fanout", duplicateViewerConn, 100*time.Millisecond)
}
func TestPremiumWeeklyXCorePassiveFanoutKeepsViewerPayloadPublic(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	purchaserEmail := "premium-purchaser@example.com"
	purchaserCookie := registerPilotWithIdentity(t, httpServer, purchaserEmail, "PremiumBuyer")
	purchaserSecondCookie := loginPilot(t, httpServer, purchaserEmail, "correct-password")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "premium-stock-viewer@example.com", "StockViewer")

	purchaserConn := dialWebSocket(t, httpServer, purchaserCookie)
	defer purchaserConn.CloseNow()
	purchaserSecondConn := dialWebSocket(t, httpServer, purchaserSecondCookie)
	defer purchaserSecondConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	readBootstrapEvents(t, purchaserConn)
	readBootstrapEvents(t, purchaserSecondConn)
	readBootstrapEvents(t, viewerConn)

	premiumPeriod := gameServer.runtime.currentPremiumPeriodKey()
	purchaseRequest := `{"request_id":"request-premium-passive-weekly-xcore","op":"premium.purchase_weekly_xcore","payload":{"product_id":"weekly_xcore","period_key":"` + premiumPeriod + `"},"client_seq":1,"v":1}`
	writeText(t, purchaserConn, purchaseRequest)
	purchaseResponse := readResponse(t, purchaserConn)
	if !purchaseResponse.OK {
		t.Fatalf("premium weekly xcore response = %+v, want success", purchaseResponse)
	}
	var purchasePayload premiumMutationPayload
	if err := json.Unmarshal(purchaseResponse.Payload, &purchasePayload); err != nil {
		t.Fatalf("decode weekly xcore: %v", err)
	}
	if purchasePayload.Wallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice || len(purchasePayload.Premium.Purchases) != 1 {
		t.Fatalf("weekly xcore payload = %+v, want purchaser debit and purchase row", purchasePayload)
	}

	purchaserStock := assertPremiumStockConsumedEvent(t, "purchaser stock", readEvent(t, purchaserConn))
	purchaserWallet := assertWalletSnapshotEvent(t, "purchaser weekly xcore wallet", readEvent(t, purchaserConn))
	if purchaserStock.StockRemaining != weeklyXCoreStockTotal-1 || purchaserWallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice {
		t.Fatalf("purchaser fanout stock/wallet = %+v/%+v, want stock decrement and wallet debit", purchaserStock, purchaserWallet)
	}
	purchaserSecondStock := assertPremiumStockConsumedEvent(t, "second purchaser stock", readEvent(t, purchaserSecondConn))
	purchaserSecondWallet := assertWalletSnapshotEvent(t, "second purchaser weekly xcore wallet", readEvent(t, purchaserSecondConn))
	if purchaserSecondStock.StockRemaining != weeklyXCoreStockTotal-1 || purchaserSecondWallet.PremiumPaid != starterWalletPremiumPaid-weeklyXCorePremiumPrice {
		t.Fatalf("second purchaser fanout stock/wallet = %+v/%+v, want stock decrement and wallet debit", purchaserSecondStock, purchaserSecondWallet)
	}
	viewerStock := assertPremiumStockConsumedEvent(t, "passive stock viewer", readEvent(t, viewerConn))
	if viewerStock.StockRemaining != weeklyXCoreStockTotal-1 || viewerStock.PeriodKey != premiumPeriod {
		t.Fatalf("passive stock payload = %+v, want public decremented stock for %s", viewerStock, premiumPeriod)
	}
	assertNoRealtimeMessageWithin(t, "passive stock viewer wallet fanout", viewerConn, 100*time.Millisecond)

	duplicateViewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer duplicateViewerConn.CloseNow()
	readBootstrapEvents(t, duplicateViewerConn)

	writeText(t, purchaserConn, purchaseRequest)
	duplicatePurchaseResponse := readResponse(t, purchaserConn)
	if !duplicatePurchaseResponse.OK {
		t.Fatalf("duplicate weekly xcore response = %+v, want cached success", duplicatePurchaseResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate weekly xcore purchaser fanout", purchaserConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate weekly xcore second purchaser fanout", purchaserSecondConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate weekly xcore viewer fanout", duplicateViewerConn, 100*time.Millisecond)
}
