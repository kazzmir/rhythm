package chart

import (
    "log"
    "io"
    "cmp"
    "fmt"
    "bufio"
    "strconv"
    "strings"
    "slices"
    "time"
)

type Chart struct {
    Metadata map[string]string
    Sections []*Section
}

type ChartEvent interface {
    CompareKey() uint64
}

type EventTimeSignature struct {
    Time uint64
    Numerator int
}

func (e *EventTimeSignature) CompareKey() uint64 {
    return e.Time
}

type EventBPMChange struct {
    Time uint64
    BPM uint64
}

func (e *EventBPMChange) CompareKey() uint64 {
    return e.Time
}

type NoteType int
const (
    NoteTypeNormal NoteType = iota
    NoteTypeStarPower
)

// a playable note in the game
type Note struct {
    Type NoteType
    Lane int
    Sustain time.Duration
    Start time.Duration
}

type EventNote struct {
    Time uint64
    Type NoteType
    Lane int
    Sustain uint64
}

func (e *EventNote) CompareKey() uint64 {
    return e.Time
}

func (char *Chart) FindSection(name string) *Section {
    for _, section := range char.Sections {
        if strings.EqualFold(section.Name, name) {
            return section
        }
    }

    return nil
}

// offset of where the song starts (in ticks?)
func (char *Chart) GetOffset() float64 {
    offsetStr, ok := char.Metadata["offset"]
    if ok {
        offset, err := strconv.ParseFloat(offsetStr, 64)
        if err == nil {
            return offset
        }
    }

    return 0
}

// offset of where the song starts (in ticks?)
func (char *Chart) GetResolution() float64 {
    offsetStr, ok := char.Metadata["resolution"]
    if ok {
        offset, err := strconv.ParseFloat(offsetStr, 64)
        if err == nil {
            return offset
        }
    }

    return 0
}

func (chart *Chart) GetSyncTrackEvents() []ChartEvent {
    section := chart.FindSection("SyncTrack")
    if section == nil {
        return nil
    }

    var events []ChartEvent

    for _, line := range section.Lines {
        // each line should look like 0 = TS 1 or 0 = B 210
        parts := strings.SplitN(line, "=", 2)
        if len(parts) != 2 {
            continue
        }
        timestampStr := strings.TrimSpace(parts[0])
        eventStr := strings.TrimSpace(parts[1])
        partsEvent := strings.Fields(eventStr)

        if len(partsEvent) != 2 {
            continue
        }

        timestamp, err := strconv.ParseUint(timestampStr, 10, 64)
        if err != nil {
            log.Printf("Invalid timestamp in SyncTrack: %s", timestampStr)
            continue
        }

        kind := partsEvent[0]
        switch strings.ToLower(kind) {
            case "ts":
                numerator, err := strconv.Atoi(partsEvent[1])
                if err != nil {
                    log.Printf("Invalid time signature numerator: %s", partsEvent[1])
                    continue
                }

                events = append(events, &EventTimeSignature{
                    Time: timestamp,
                    Numerator: numerator,
                })
            case "b":
                bpmValue, err := strconv.ParseUint(partsEvent[1], 10, 64)
                if err != nil {
                    log.Printf("Invalid BPM value: %s", partsEvent[1])
                    continue
                }

                events = append(events, &EventBPMChange{
                    Time: timestamp,
                    BPM: bpmValue,
                })
            default:
                log.Printf("Unknown SyncTrack event type: %s", kind)
        }

    }

    return events
}

func (chart *Chart) GetNoteEvents(kind string) []EventNote {
    // there are other kinds like Drums, DoubleBass, DoubleRhythm
    section := chart.FindSection(fmt.Sprintf("%vSingle", kind))
    if section == nil {
        return nil
    }

    convertNoteType := func(noteType string) NoteType {
        switch strings.ToLower(noteType) {
            case "n": return NoteTypeNormal
            case "s": return NoteTypeStarPower
        }

        return NoteTypeNormal
    }

    var events []EventNote

    for _, line := range section.Lines {
        parts := strings.SplitN(line, "=", 2)
        if len(parts) != 2 {
            continue
        }
        // expect 300 = N 2 0
        timestampStr := strings.TrimSpace(parts[0])
        noteStr := strings.TrimSpace(parts[1])

        partsNote := strings.Fields(noteStr)

        if len(partsNote) < 3 {
            continue
        }

        timestamp, err := strconv.ParseUint(timestampStr, 10, 64)

        if err != nil {
            log.Printf("Invalid timestamp in %s: %s", kind, timestampStr)
            continue
        }

        noteType := partsNote[0]

        lane, err := strconv.Atoi(partsNote[1])
        if err != nil {
            log.Printf("Invalid lane in %s: %s", kind, partsNote[2])
            continue
        }

        var sustain uint64 = 0
        sustain, err = strconv.ParseUint(partsNote[2], 10, 64)
        if err != nil {
            log.Printf("Invalid sustain in %s: %s", kind, partsNote[3])
            continue
        }

        events = append(events, EventNote{
            Time: timestamp,
            Type: convertNoteType(noteType),
            Lane: lane,
            Sustain: sustain,
        })
    }

    return events
}

func (chart *Chart) GetEvents(kind string) []ChartEvent {
    syncEvents := chart.GetSyncTrackEvents()
    noteEvents := chart.GetNoteEvents(kind)

    // FIXME: include the Events section

    var all []ChartEvent

    all = append(all, syncEvents...)
    for _, note := range noteEvents {
        all = append(all, &note)
    }

    slices.SortFunc(all, func(a, b ChartEvent) int {
        return cmp.Compare(a.CompareKey(), b.CompareKey())
    })

    return all
}

func (chart *Chart) GetNotes(kind string) []Note {
    resolution := chart.GetResolution()
    if resolution < 1 {
        resolution = 1
    }

    var notes []Note

    // FIXME: what is the default signature?
    timeSignature := 4
    var currentTime time.Duration
    currentTick := uint64(0)
    beatsPerMinute := uint64(120_000)

    millisecondsPerBeat := func() float64 {
        // beats per minute is in microseconds
        return float64(timeSignature * 60 * 1000) / (float64(beatsPerMinute) / 1000)
    }

    millisecondsPerTick := func() float64 {
        return millisecondsPerBeat() / (resolution * float64(timeSignature))
    }

    updateTime := func(newTick uint64) {
        diff := newTick - currentTick
        millis := millisecondsPerTick() * float64(diff)
        currentTime += time.Duration(millis * 1000) * time.Microsecond
        currentTick = newTick
    }

    for _, eventRaw := range chart.GetEvents(kind) {
        /*
        if i > 40 {
            break
        }
        fmt.Printf("Current tick: %d time: %v ms per beat=%v ms per tick=%v\n", currentTick, currentTime, millisecondsPerBeat(), millisecondsPerTick())
        */
        switch event := eventRaw.(type) {
            case *EventTimeSignature:
                updateTime(event.Time)
                timeSignature = event.Numerator
            case *EventBPMChange:
                updateTime(event.Time)
                beatsPerMinute = event.BPM
            case *EventNote:
                updateTime(event.Time)

                sustainTime := time.Duration(float64(event.Sustain) * millisecondsPerTick() * 1e3) * time.Microsecond

                note := Note{
                    Type: event.Type,
                    Lane: event.Lane,
                    Start: currentTime,
                    Sustain: sustainTime,
                }
                notes = append(notes, note)
        }
    }

    return notes
}

func (chart *Chart) GetDifficulties(kind string) []string {
    var out []string
    for _, section := range chart.Sections {
        if strings.HasSuffix(strings.ToLower(section.Name), strings.ToLower(kind)) {
            firstPart := section.Name[:len(section.Name)-len(kind)]
            out = append(out, firstPart)
        }
    }

    return out
}

type ParseState int
const (
    ParseTop ParseState = iota
    ParseSectionStart
    ParseSectionBody
)

type Section struct {
    Name string
    Lines []string
}

func ParseChart(reader io.Reader) (*Chart, error) {
    scanner := bufio.NewScanner(reader)

    state := ParseTop

    var currentSection *Section

    var sections []*Section

    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        // fmt.Printf("Line: '%v'\n", line)

        switch state {
            case ParseTop:
                // fmt.Printf("Top: '%v'\n", line)
                index := strings.Index(line, "[")
                if index != -1 {
                    state = ParseSectionStart

                    name := strings.Trim(line[index:], "[]")

                    currentSection = &Section{
                        // Name: strings.Trim(line, "[]"),
                        Name: name,
                    }
                }
            case ParseSectionStart:
                if line == "[" {
                    return nil, fmt.Errorf("unexpected '[' while parsing section start")
                }

                if line == "{" {
                    state = ParseSectionBody
                }
            case ParseSectionBody:
                if line == "}" {
                    state = ParseTop
                    sections = append(sections, currentSection)
                    currentSection = nil
                } else {
                    currentSection.Lines = append(currentSection.Lines, line)
                }
        }
    }

    var chart Chart
    chart.Metadata = make(map[string]string)
    chart.Sections = sections

    metadata := chart.FindSection("Song")
    if metadata != nil {
        for _, line := range metadata.Lines {
            parts := strings.SplitN(line, "=", 2)
            if len(parts) != 2 {
                continue
            }
            key := strings.TrimSpace(parts[0])
            value := strings.TrimSpace(parts[1])
            chart.Metadata[strings.ToLower(key)] = value
        }
    }


        /*
    for _, section := range sections {
        fmt.Printf("Section: %s lines %d\n", section.Name, len(section.Lines))

        // for debugging
        else if section.Name == "SyncTrack" {
            // every line should be 0 = TS 4 or 0 = B 210
            for _, line := range section.Lines {
                parts := strings.SplitN(line, "=", 2)
                if len(parts) != 2 {
                    continue
                }
                timestampStr := strings.TrimSpace(parts[0])
                eventStr := strings.TrimSpace(parts[1])
                fmt.Printf("SyncTrack Event: %s -> %s\n", timestampStr, eventStr)
            }
        } else if section.Name == "Events" {
            for _, line := range section.Lines {
                parts := strings.SplitN(line, "=", 2)
                // 800 = E "section intro"
                if len(parts) != 2 {
                    continue
                }
                timestampStr := strings.TrimSpace(parts[0])
                eventStr := strings.TrimSpace(parts[1])
                fmt.Printf("Event: %s -> %s\n", timestampStr, eventStr)
            }
        } else {
            // should be something like ExpertSingle
            for _, line := range section.Lines {
                parts := strings.SplitN(line, "=", 2)
                if len(parts) != 2 {
                    continue
                }
                // expect 300 = N 2 0
                timestampStr := strings.TrimSpace(parts[0])
                noteStr := strings.TrimSpace(parts[1])
                fmt.Printf("Note in %s: %s -> %s\n", section.Name, timestampStr, noteStr)
            }
        }
    }
        */

    return &chart, nil
}
