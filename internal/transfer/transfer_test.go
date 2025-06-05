package transfer

import "testing"

func TestTransferCancel(t *testing.T) {
	tm := NewTransferManager(TransferOptions{})
	tm.Start()
	tm.Stop()
	<-tm.done
	stats := tm.GetStats()
	if stats.Status != StatusCancelled {
		t.Fatalf("expected %s, got %s", StatusCancelled, stats.Status)
	}
}
