package chart

import (
    "io"
    "fmt"
    "bufio"
    "strings"
)

type Chart struct {
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

    for _, section := range sections {
        fmt.Printf("Section: %s lines %d\n", section.Name, len(section.Lines))
    }

    return nil, scanner.Err()
}
