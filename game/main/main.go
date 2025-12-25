package main

import (
    "log"
    "time"
    "image/color"

    smflib "gitlab.com/gomidi/midi/v2/smf"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
    "github.com/hajimehoshi/ebiten/v2/vector"
)

type Note struct {
    Start time.Duration
    End time.Duration
}

type Fret struct {
    InUse bool
    Notes []Note
}

type Engine struct {
    Frets []Fret
    StartTime time.Time
}

func MakeEngine(midFile string) *Engine {
    engine := &Engine{
        Frets: make([]Fret, 6),
    }

    // keys := make(map[uint8]*Fret)

    difficulty := "medium"
    var low, high int
    switch difficulty {
        case "easy":
            low = 60
            high = 65
        case "medium":
            low = 72
            high = 76
        case "hard":
            high = 84
            low = 90
        case "expert":
            high = 96
            low = 100
    }

    reader := smflib.ReadTracks("notes.mid", 2)
    reader.Do(func (event smflib.TrackEvent) {
        // log.Printf("Tick: %d, Microseconds: %v, Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.Message)
        var channel, key, velocity uint8
        if event.Message.GetNoteOn(&channel, &key, &velocity) {
            if int(key) >= low && int(key) <= high {
                log.Printf("Tick: %d, Microseconds: %v, Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.Message)
                if velocity > 0 {
                    useFret := int(key) - low
                    fret := &engine.Frets[useFret]
                    fret.Notes = append(fret.Notes, Note{
                        Start: time.Microsecond * time.Duration(event.AbsMicroSeconds),
                    })
                } else {
                    useFret := int(key) - low
                    fret := &engine.Frets[useFret]
                    if len(fret.Notes) > 0 {
                        lastNote := &fret.Notes[len(fret.Notes)-1]
                        lastNote.End = time.Microsecond * time.Duration(event.AbsMicroSeconds)
                    }
                }
            }
        }
    })

    return engine
}

func (engine *Engine) Update() error {
    if engine.StartTime.IsZero() {
        engine.StartTime = time.Now()
    }

    keys := inpututil.AppendJustPressedKeys(nil)
    for _, key := range keys {
        switch key {
            case ebiten.KeyEscape, ebiten.KeyCapsLock:
                return ebiten.Termination
        }
    }

    return nil
}

func (engine *Engine) Draw(screen *ebiten.Image) {
    delta := time.Since(engine.StartTime)
    for i, fret := range engine.Frets {
        fretColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
        switch i {
            case 0: fretColor = color.RGBA{R: 0, G: 255, B: 0, A: 255} // Green
            case 1: fretColor = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red
            case 2: fretColor = color.RGBA{R: 255, G: 255, B: 0, A: 255} // Yellow
            case 3: fretColor = color.RGBA{R: 0, G: 0, B: 255, A: 255} // Blue
            case 4: fretColor = color.RGBA{R: 255, G: 165, B: 0, A: 255} // Orange
            case 5: fretColor = color.RGBA{R: 128, G: 0, B: 128, A: 255} // Purple
        }

        for _, note := range fret.Notes {
            start := int64((note.Start - delta) / time.Millisecond) / 10
            end := int64((note.End - delta) / time.Millisecond) / 10

            for t := start; t <= end; t += 10 {
                if t < 800 && t > -100 {
                    x := 20 + t
                    y := 100 + i * 60

                    vector.FillCircle(screen, float32(x), float32(y), 15, fretColor, true)
                }
            }
        }
    }
}

func (engine *Engine) Layout(outsideWidth, outsideHeight int) (int, int) {
    return outsideWidth, outsideHeight
}

func main() {
    /*
    smf, err := smflib.ReadFile("notes.mid")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Number of tracks: %d", len(smf.Tracks))
    */

    /*
    reader := smflib.ReadTracks("notes.mid", 2)
    reader.Do(func (event smflib.TrackEvent) {
        log.Printf("Tick: %d, Microseconds: %v, Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.Message)
    })
    */

    log.SetFlags(log.Ldate | log.Lshortfile | log.Lmicroseconds)

    log.Printf("Initializing")

    ebiten.SetWindowSize(800, 600)
    ebiten.SetWindowTitle("Rhythm Game")

    engine := MakeEngine("notes.mid")

    ebiten.RunGame(engine)

    log.Printf("Bye")
}
