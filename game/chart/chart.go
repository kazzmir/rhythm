package chart

import (
    "io"
    "fmt"
    "bufio"
    "strings"
)

type Chart struct {
}

func ParseChart(reader io.Reader) (*Chart, error) {
    scanner := bufio.NewScanner(reader)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        fmt.Println(line)
    }

    return nil, scanner.Err()
}
