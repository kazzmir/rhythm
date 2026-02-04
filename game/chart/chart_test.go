package chart

import (
    "testing"
    "strings"
)

func TestSyncTrackEvents(test *testing.T) {
    input := `[SyncTrack]
    {
        0 = TS 4
        1920 = B 210000
        3840 = TS 3
    }
    `

    chart, err := ParseChart(strings.NewReader(input))
    if err != nil {
        test.Fatalf("Failed to parse chart: %v", err)
    }

    events := chart.GetSyncTrackEvents()
    if len(events) != 3 {
        test.Fatalf("Expected 3 sync track events, got %d", len(events))
    }

    if tsEvent, ok := events[0].(*EventTimeSignature); !ok || tsEvent.Numerator != 4 || tsEvent.Time != 0 {
        test.Errorf("First event incorrect, got %+v", events[0])
    }

    if bpmEvent, ok := events[1].(*EventBPMChange); !ok || bpmEvent.BPM != 210000 || bpmEvent.Time != 1920 {
        test.Errorf("Second event incorrect, got %+v", events[1])
    }

    if tsEvent, ok := events[2].(*EventTimeSignature); !ok || tsEvent.Numerator != 3 || tsEvent.Time != 3840 {
        test.Errorf("Third event incorrect, got %+v", events[2])
    }
}
