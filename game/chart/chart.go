package chart

import (
    "log"
    "io"
    "fmt"
    "bufio"
    "strconv"
    "strings"
)

type Chart struct {
    Metadata map[string]string
    Sections []*Section
}

type ChartEvent interface {
}

type EventTimeSignature struct {
    Time uint64
    Numerator int
}

type EventBPMChange struct {
    Time uint64
    BPM uint64
}

type EventNote struct {
    Time uint64
    Type int
    Lane int
    Sustain uint64
}

func (char *Chart) FindSection(name string) *Section {
    for _, section := range char.Sections {
        if strings.ToLower(section.Name) == strings.ToLower(name) {
            return section
        }
    }

    return nil
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

func (chart *Chart) GetNoteEvents() []EventNote {
    return nil
}

func (chart *Chart) GetEvents() {
    syncEvents := chart.GetSyncTrackEvents()
    noteEvents := chart.GetNoteEvents()

    _ = syncEvents
    _ = noteEvents

    // merge the two by sorting them by time

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
        // fmt.Println(line)

        switch state {
            case ParseTop:
                if strings.HasPrefix(line, "[") {
                    state = ParseSectionStart
                    currentSection = &Section{
                        Name: strings.Trim(line, "[]"),
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

    for _, section := range sections {
        fmt.Printf("Section: %s lines %d\n", section.Name, len(section.Lines))

        if section.Name == "Song" {
            for _, line := range section.Lines {
                parts := strings.SplitN(line, "=", 2)
                if len(parts) != 2 {
                    continue
                }
                key := strings.TrimSpace(parts[0])
                value := strings.TrimSpace(parts[1])
                chart.Metadata[key] = value
            }
        } else if section.Name == "SyncTrack" {
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

    return &chart, nil
}
