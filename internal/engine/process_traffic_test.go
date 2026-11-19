package engine

import (
	"testing"
	"time"
)

func TestProcessRatesCountLiveAndRecentlyClosedFlows(t *testing.T) {
	b := &Balancer{}
	first, finish := b.trackProcess(12)
	second, finishSecond := b.trackProcess(12)
	defer finishSecond()
	other, finishOther := b.trackProcess(23)
	defer finishOther()
	first.add(250_000)
	second.add(250_000)
	finish()
	other.add(1_000_000)
	now := b.processTraffic.last.Add(2 * time.Second)
	if rates := b.ProcessRates(now); rates[12] != 2 || rates[23] != 4 {
		t.Fatalf("unexpected rates: %v", rates)
	}
	second.add(250_000)
	if rates := b.ProcessRates(now.Add(time.Second)); rates[12] != 2 || rates[23] != 0 {
		t.Fatalf("unexpected next sample: %v", rates)
	}
}
