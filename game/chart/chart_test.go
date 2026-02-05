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

func TestEventNotes(test *testing.T) {
    input := `[ExpertSingle]
    {
        0 = N 0 192
        1920 = N 1 192
        3840 = N 2 192
        4000 = S 3 96
    }
    `

    chart, err := ParseChart(strings.NewReader(input))
    if err != nil {
        test.Fatalf("Failed to parse chart: %v", err)
    }

    events := chart.GetNoteEvents("Expert")
    if len(events) != 4 {
        test.Fatalf("Expected 4 note events, got %d", len(events))
    }

    if events[0].Time != 0 || events[0].Lane != 0 || events[0].Sustain != 192 || events[0].Type != NoteTypeNormal {
        test.Errorf("First note event incorrect, got %+v", events[0])
    }

    if events[1].Time != 1920 || events[1].Lane != 1 || events[1].Sustain != 192 || events[1].Type != NoteTypeNormal {
        test.Errorf("Second note event incorrect, got %+v", events[1])
    }

    if events[2].Time != 3840 || events[2].Lane != 2 || events[2].Sustain != 192 || events[2].Type != NoteTypeNormal {
        test.Errorf("Third note event incorrect, got %+v", events[2])
    }

    if events[3].Time != 4000 || events[3].Lane != 3 || events[3].Sustain != 96 || events[3].Type != NoteTypeStarPower {
        test.Errorf("Fourth note event incorrect, got %+v", events[3])
    }
}

func TestAllEvents(test *testing.T) {
    input := `[SyncTrack]
    {
        0 = TS 4
        1920 = B 210000
        3000 = TS 3
    }
    [ExpertSingle]
    {
        0 = N 0 192
        800 = N 1 192
        1920 = S 2 96
        2000 = N 3 192
    }
    `

    chart, err := ParseChart(strings.NewReader(input))
    if err != nil {
        test.Fatalf("Failed to parse chart: %v", err)
    }

    events := chart.GetEvents("Expert")
    if len(events) != 7 {
        test.Fatalf("Expected 7 total events, got %d", len(events))
    }

    note := events[2].(*EventNote)
    if note.Time != 800 || note.Lane != 1 || note.Sustain != 192 || note.Type != NoteTypeNormal {
        test.Errorf("First event incorrect, got %+v", events[2])
    }
}
