package main

import (
    "log"

    smflib "gitlab.com/gomidi/midi/v2/smf"
)

func main() {
    smf, err := smflib.ReadFile("notes.mid")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Number of tracks: %d", len(smf.Tracks))
}
