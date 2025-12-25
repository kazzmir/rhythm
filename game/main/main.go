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

const ScreenWidth = 1024
const ScreenHeight = 768

type Note struct {
    Start time.Duration
    End time.Duration
}

type Fret struct {
    InUse bool
    Notes []Note

    // time when this key was pressed (or zero if not pressed)
    Press time.Time
    Key ebiten.Key
}

type Engine struct {
    Frets []Fret
    StartTime time.Time
}

func MakeEngine(midFile string) *Engine {
    engine := &Engine{
        Frets: make([]Fret, 6),
    }

    engine.Frets[0].Key = ebiten.Key1
    engine.Frets[1].Key = ebiten.Key2
    engine.Frets[2].Key = ebiten.Key3
    engine.Frets[3].Key = ebiten.Key4
    engine.Frets[4].Key = ebiten.Key5
    engine.Frets[5].Key = ebiten.Key6

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

    for i := range engine.Frets {
        fret := &engine.Frets[i]
        fret.Press = time.Time{}
        if ebiten.IsKeyPressed(fret.Key) {
            fret.Press = time.Now()
        }
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

    playLine := 80

    vector.StrokeLine(screen, float32(playLine), 20, float32(playLine), 500, 3, color.RGBA{R: 255, G: 255, B: 255, A: 255}, true)

    delta := time.Since(engine.StartTime)
    for i, fret := range engine.Frets {

        yFret := 100 + i * 60

        vector.StrokeLine(screen, 0, float32(yFret), float32(ScreenWidth), float32(yFret), 2, color.RGBA{R: 200, G: 200, B: 200, A: 255}, true)

        fretColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
        switch i {
            case 0: fretColor = color.NRGBA{R: 0, G: 255, B: 0, A: 255} // Green
            case 1: fretColor = color.NRGBA{R: 255, G: 0, B: 0, A: 255} // Red
            case 2: fretColor = color.NRGBA{R: 255, G: 255, B: 0, A: 255} // Yellow
            case 3: fretColor = color.NRGBA{R: 0, G: 0, B: 255, A: 255} // Blue
            case 4: fretColor = color.NRGBA{R: 255, G: 165, B: 0, A: 255} // Orange
            case 5: fretColor = color.NRGBA{R: 128, G: 0, B: 128, A: 255} // Purple
        }

        transparent := fretColor
        transparent.A = 100
        vector.FillCircle(screen, float32(playLine), float32(yFret), 20, transparent, false)

        if !fret.Press.IsZero() {
            vector.StrokeCircle(screen, float32(playLine), float32(yFret), 21, 2, fretColor, false)
        }

        for _, note := range fret.Notes {
            start := int64((note.Start - delta) / time.Millisecond) / 10
            end := int64((note.End - delta) / time.Millisecond) / 10

            for t := start; t <= end; t += 10 {
                if t < ScreenWidth + 20 && t > -100 {
                    x := playLine + int(t)

                    vector.FillCircle(screen, float32(x), float32(yFret), 15, fretColor, true)
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

    ebiten.SetWindowSize(ScreenWidth, ScreenHeight)
    ebiten.SetWindowTitle("Rhythm Game")

    engine := MakeEngine("notes.mid")

    ebiten.RunGame(engine)

    log.Printf("Bye")
}
