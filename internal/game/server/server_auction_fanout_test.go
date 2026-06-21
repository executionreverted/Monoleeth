package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/realtime"
)

func TestAuctionBidPassiveFanoutNotifiesBidderPreviousBidderAndViewer(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	previousCookie := registerPilotWithIdentity(t, httpServer, "previous-bidder@example.com", "PrevBidder")
	bidderCookie := registerPilotWithIdentity(t, httpServer, "new-bidder@example.com", "NewBidder")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "auction-viewer@example.com", "AuctionViewer")

	previousConn := dialWebSocket(t, httpServer, previousCookie)
	defer previousConn.CloseNow()
	bidderConn := dialWebSocket(t, httpServer, bidderCookie)
	defer bidderConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	previousBootstrap := readBootstrapEvents(t, previousConn)
	bidderBootstrap := readBootstrapEvents(t, bidderConn)
	viewerBootstrap := readBootstrapEvents(t, viewerConn)
	previousSeq := previousBootstrap[len(previousBootstrap)-1].Sequence
	bidderSeq := bidderBootstrap[len(bidderBootstrap)-1].Sequence
	viewerSeq := viewerBootstrap[len(viewerBootstrap)-1].Sequence

	previousBidRequest := `{"request_id":"request-auction-passive-previous-bid","op":"auction.bid","payload":{"auction_id":"` + seedAuctionID.String() + `","amount":300},"client_seq":1,"v":1}`
	writeText(t, previousConn, previousBidRequest)
	previousBidResponse := readResponse(t, previousConn)
	if !previousBidResponse.OK {
		t.Fatalf("previous bid response = %+v, want success", previousBidResponse)
	}
	previousBidPlaced := assertAuctionLotEvent(t, "previous bidder bid placed", readEvent(t, previousConn), realtime.EventAuctionBidPlaced)
	previousLotUpdated := assertAuctionLotEvent(t, "previous bidder lot updated", readEvent(t, previousConn), realtime.EventAuctionLotUpdated)
	previousWallet := assertWalletSnapshotEvent(t, "previous bidder wallet", readEvent(t, previousConn))
	if !previousBidPlaced.Leading || !previousLotUpdated.Leading || previousWallet.Credits != starterWalletCredits-300 {
		t.Fatalf("previous bid events = %+v/%+v wallet=%+v, want leading with debit", previousBidPlaced, previousLotUpdated, previousWallet)
	}
	if previousBidPlaced.CurrentBid != 300 || previousBidPlaced.Status != auction.LotStatusActive.String() {
		t.Fatalf("previous bid lot = %+v, want active 300 bid", previousBidPlaced)
	}
	if previousBidPlaced.Sequence != previousSeq+1 || previousLotUpdated.Sequence != previousSeq+2 {
		t.Fatalf("previous bid seq = %d/%d after %d, want contiguous", previousBidPlaced.Sequence, previousLotUpdated.Sequence, previousSeq)
	}
	previousSeq += 3

	bidderPassive := assertAuctionLotEvent(t, "new bidder passive first bid", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	if bidderPassive.Leading || bidderPassive.CurrentBid != 300 {
		t.Fatalf("new bidder passive lot = %+v, want public non-leading 300 bid", bidderPassive)
	}
	if bidderPassive.Sequence != bidderSeq+1 {
		t.Fatalf("new bidder passive seq = %d after %d, want contiguous", bidderPassive.Sequence, bidderSeq)
	}
	bidderSeq = bidderPassive.Sequence

	viewerPassive := assertAuctionLotEvent(t, "viewer passive first bid", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerPassive.Leading || viewerPassive.CurrentBid != 300 {
		t.Fatalf("viewer passive lot = %+v, want public non-leading 300 bid", viewerPassive)
	}
	if viewerPassive.Sequence != viewerSeq+1 {
		t.Fatalf("viewer passive seq = %d after %d, want contiguous", viewerPassive.Sequence, viewerSeq)
	}
	viewerSeq = viewerPassive.Sequence

	newBidRequest := `{"request_id":"request-auction-passive-new-bid","op":"auction.bid","payload":{"auction_id":"` + seedAuctionID.String() + `","amount":450},"client_seq":2,"v":1}`
	writeText(t, bidderConn, newBidRequest)
	newBidResponse := readResponse(t, bidderConn)
	if !newBidResponse.OK {
		t.Fatalf("new bid response = %+v, want success", newBidResponse)
	}
	var newBidPayload auctionMutationPayload
	if err := json.Unmarshal(newBidResponse.Payload, &newBidPayload); err != nil {
		t.Fatalf("decode new bid response: %v", err)
	}
	if newBidPayload.Wallet.Credits != starterWalletCredits-450 || !newBidPayload.Lot.Leading {
		t.Fatalf("new bid response payload = %+v, want bidder leading with wallet debit", newBidPayload)
	}

	newBidPlaced := assertAuctionLotEvent(t, "new bidder bid placed", readEvent(t, bidderConn), realtime.EventAuctionBidPlaced)
	newBidUpdated := assertAuctionLotEvent(t, "new bidder lot updated", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	newBidWallet := assertWalletSnapshotEvent(t, "new bidder wallet", readEvent(t, bidderConn))
	if !newBidPlaced.Leading || !newBidUpdated.Leading || newBidWallet.Credits != starterWalletCredits-450 {
		t.Fatalf("new bidder events = %+v/%+v wallet=%+v, want leading with wallet debit", newBidPlaced, newBidUpdated, newBidWallet)
	}
	if newBidPlaced.CurrentBid != 450 || newBidUpdated.CurrentBid != 450 {
		t.Fatalf("new bidder lot events = %+v/%+v, want current bid 450", newBidPlaced, newBidUpdated)
	}
	if newBidPlaced.Sequence != bidderSeq+1 || newBidUpdated.Sequence != bidderSeq+2 || newBidWallet.Sequence != bidderSeq+3 {
		t.Fatalf("new bidder seq = %d/%d/%d after %d, want contiguous", newBidPlaced.Sequence, newBidUpdated.Sequence, newBidWallet.Sequence, bidderSeq)
	}
	bidderSeq = newBidWallet.Sequence

	previousOutbid := assertAuctionLotEvent(t, "previous bidder outbid", readEvent(t, previousConn), realtime.EventAuctionLotUpdated)
	previousRefundWallet := assertWalletSnapshotEvent(t, "previous bidder refund wallet", readEvent(t, previousConn))
	if previousOutbid.Leading || previousOutbid.CurrentBid != 450 || previousRefundWallet.Credits != starterWalletCredits {
		t.Fatalf("previous outbid = %+v wallet=%+v, want non-leading refund", previousOutbid, previousRefundWallet)
	}
	if previousOutbid.Sequence != previousSeq+1 || previousRefundWallet.Sequence != previousSeq+2 {
		t.Fatalf("previous refund seq = %d/%d after %d, want contiguous", previousOutbid.Sequence, previousRefundWallet.Sequence, previousSeq)
	}

	viewerOutbid := assertAuctionLotEvent(t, "viewer passive outbid", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerOutbid.Leading || viewerOutbid.CurrentBid != 450 {
		t.Fatalf("viewer outbid lot = %+v, want public non-leading 450 bid", viewerOutbid)
	}
	if viewerOutbid.Sequence != viewerSeq+1 {
		t.Fatalf("viewer outbid seq = %d after %d, want contiguous", viewerOutbid.Sequence, viewerSeq)
	}

	writeText(t, bidderConn, newBidRequest)
	duplicateBidResponse := readResponse(t, bidderConn)
	if !duplicateBidResponse.OK {
		t.Fatalf("duplicate bid response = %+v, want cached success", duplicateBidResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate bid bidder fanout", bidderConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate bid previous fanout", previousConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate bid viewer fanout", viewerConn, 100*time.Millisecond)
}
func TestAuctionBuyNowPassiveFanoutKeepsGrantPrivate(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	bidderCookie := registerPilotWithIdentity(t, httpServer, "current-bidder@example.com", "CurrentBidder")
	buyerCookie := registerPilotWithIdentity(t, httpServer, "buy-now-buyer@example.com", "BuyNowBuyer")
	viewerCookie := registerPilotWithIdentity(t, httpServer, "buy-now-viewer@example.com", "BuyNowViewer")

	bidderConn := dialWebSocket(t, httpServer, bidderCookie)
	defer bidderConn.CloseNow()
	buyerConn := dialWebSocket(t, httpServer, buyerCookie)
	defer buyerConn.CloseNow()
	viewerConn := dialWebSocket(t, httpServer, viewerCookie)
	defer viewerConn.CloseNow()
	bidderBootstrap := readBootstrapEvents(t, bidderConn)
	buyerBootstrap := readBootstrapEvents(t, buyerConn)
	viewerBootstrap := readBootstrapEvents(t, viewerConn)
	bidderSeq := bidderBootstrap[len(bidderBootstrap)-1].Sequence
	buyerSeq := buyerBootstrap[len(buyerBootstrap)-1].Sequence
	viewerSeq := viewerBootstrap[len(viewerBootstrap)-1].Sequence

	writeText(t, bidderConn, `{"request_id":"request-auction-buy-now-current-bid","op":"auction.bid","payload":{"auction_id":"`+seedAuctionID.String()+`","amount":300},"client_seq":1,"v":1}`)
	currentBidResponse := readResponse(t, bidderConn)
	if !currentBidResponse.OK {
		t.Fatalf("current bid response = %+v, want success", currentBidResponse)
	}
	currentBidPlaced := assertAuctionLotEvent(t, "current bidder bid placed", readEvent(t, bidderConn), realtime.EventAuctionBidPlaced)
	currentBidUpdated := assertAuctionLotEvent(t, "current bidder lot updated", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	currentBidWallet := assertWalletSnapshotEvent(t, "current bidder bid wallet", readEvent(t, bidderConn))
	if !currentBidPlaced.Leading || !currentBidUpdated.Leading || currentBidWallet.Credits != starterWalletCredits-300 {
		t.Fatalf("current bid events = %+v/%+v wallet=%+v, want leading with debit", currentBidPlaced, currentBidUpdated, currentBidWallet)
	}
	bidderSeq += 3
	buyerBidView := assertAuctionLotEvent(t, "buyer passive bid", readEvent(t, buyerConn), realtime.EventAuctionLotUpdated)
	if buyerBidView.Leading || buyerBidView.CurrentBid != 300 {
		t.Fatalf("buyer passive bid lot = %+v, want public non-leading 300 bid", buyerBidView)
	}
	buyerSeq = buyerBidView.Sequence
	viewerBidView := assertAuctionLotEvent(t, "viewer passive bid", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerBidView.Leading || viewerBidView.CurrentBid != 300 {
		t.Fatalf("viewer passive bid lot = %+v, want public non-leading 300 bid", viewerBidView)
	}
	viewerSeq = viewerBidView.Sequence

	buyNowRequest := `{"request_id":"request-auction-passive-buy-now","op":"auction.buy_now","payload":{"auction_id":"` + seedAuctionID.String() + `"},"client_seq":2,"v":1}`
	writeText(t, buyerConn, buyNowRequest)
	buyNowResponse := readResponse(t, buyerConn)
	if !buyNowResponse.OK {
		t.Fatalf("buy now response = %+v, want success", buyNowResponse)
	}
	var buyNowPayload auctionMutationPayload
	if err := json.Unmarshal(buyNowResponse.Payload, &buyNowPayload); err != nil {
		t.Fatalf("decode buy now response: %v", err)
	}
	if buyNowPayload.Price != 650 || buyNowPayload.Grant == nil || buyNowPayload.Wallet.Credits != starterWalletCredits-650 {
		t.Fatalf("buy now payload = %+v, want private grant and buyer debit", buyNowPayload)
	}

	buyerClosed := assertAuctionClosedEvent(t, "buyer closed", readEvent(t, buyerConn))
	buyerLotUpdated := assertAuctionLotEvent(t, "buyer lot updated", readEvent(t, buyerConn), realtime.EventAuctionLotUpdated)
	buyerWallet := assertWalletSnapshotEvent(t, "buyer wallet", readEvent(t, buyerConn))
	if buyerClosed.Grant == nil || buyerClosed.Lot.Status != auction.LotStatusClosed.String() || buyerClosed.Lot.Leading {
		t.Fatalf("buyer closed event = %+v, want closed private grant without leading", buyerClosed)
	}
	if buyerLotUpdated.Status != auction.LotStatusClosed.String() || buyerLotUpdated.Leading || buyerWallet.Credits != starterWalletCredits-650 {
		t.Fatalf("buyer lot/wallet = %+v/%+v, want closed non-leading with debit", buyerLotUpdated, buyerWallet)
	}
	if buyerClosed.Sequence != buyerSeq+1 || buyerLotUpdated.Sequence != buyerSeq+2 || buyerWallet.Sequence != buyerSeq+3 {
		t.Fatalf("buyer buy-now seq = %d/%d/%d after %d, want contiguous", buyerClosed.Sequence, buyerLotUpdated.Sequence, buyerWallet.Sequence, buyerSeq)
	}

	refundedLot := assertAuctionLotEvent(t, "refunded bidder lot", readEvent(t, bidderConn), realtime.EventAuctionLotUpdated)
	refundedWallet := assertWalletSnapshotEvent(t, "refunded bidder wallet", readEvent(t, bidderConn))
	if refundedLot.Status != auction.LotStatusClosed.String() || refundedLot.Leading || refundedWallet.Credits != starterWalletCredits {
		t.Fatalf("refunded bidder events = %+v wallet=%+v, want public closed lot and refund", refundedLot, refundedWallet)
	}
	if refundedLot.Sequence != bidderSeq+1 || refundedWallet.Sequence != bidderSeq+2 {
		t.Fatalf("refunded bidder seq = %d/%d after %d, want contiguous", refundedLot.Sequence, refundedWallet.Sequence, bidderSeq)
	}

	viewerClosedLot := assertAuctionLotEvent(t, "passive viewer closed lot", readEvent(t, viewerConn), realtime.EventAuctionLotUpdated)
	if viewerClosedLot.Status != auction.LotStatusClosed.String() || viewerClosedLot.Leading {
		t.Fatalf("viewer closed lot = %+v, want public closed non-leading lot", viewerClosedLot)
	}
	if viewerClosedLot.Sequence != viewerSeq+1 {
		t.Fatalf("viewer closed seq = %d after %d, want contiguous", viewerClosedLot.Sequence, viewerSeq)
	}

	writeText(t, buyerConn, buyNowRequest)
	duplicateBuyNowResponse := readResponse(t, buyerConn)
	if !duplicateBuyNowResponse.OK {
		t.Fatalf("duplicate buy-now response = %+v, want cached success", duplicateBuyNowResponse)
	}
	assertNoRealtimeMessageWithin(t, "duplicate buy-now buyer fanout", buyerConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate buy-now bidder fanout", bidderConn, 100*time.Millisecond)
	assertNoRealtimeMessageWithin(t, "duplicate buy-now viewer fanout", viewerConn, 100*time.Millisecond)
}
