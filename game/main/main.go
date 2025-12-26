package main

import (
    "log"
    "time"
    "os"
    "fmt"
    "image/color"
    "image/png"
    "path/filepath"
    "sync"

    smflib "gitlab.com/gomidi/midi/v2/smf"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/audio"
    "github.com/hajimehoshi/ebiten/v2/audio/vorbis"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
    "github.com/hajimehoshi/ebiten/v2/vector"
    "github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const ScreenWidth = 1200
const ScreenHeight = 800

const NoteThresholdHigh = time.Millisecond * 250
const NoteThresholdLow = -time.Millisecond * 150

type NoteState int
const (
    NoteStatePending NoteState = iota
    NoteStateHit
    NoteStateMissed
)

type Note struct {
    Start time.Duration
    End time.Duration
    State NoteState
}

type Fret struct {
    InUse bool
    Notes []Note

    // the index of the next note to be hit
    StartNote int

    // time when this key was pressed (or zero if not pressed)
    Press time.Time
    Key ebiten.Key
}

type Song struct {
    Frets []Fret
    StartTime time.Time
    CleanupFuncs []func()

    Song *audio.Player
    Guitar *audio.Player
    DoSong sync.Once
    NotesHit int
    NotesMissed int
}

func (song *Song) Close() {
    for _, cleanup := range song.CleanupFuncs {
        cleanup()
    }
}

func (song *Song) Update() {
    song.DoSong.Do(func(){
        song.Song.Play()
        // engine.Guitar.Play()
    })

    if song.StartTime.IsZero() {
        song.StartTime = time.Now()
    }

    for i := range song.Frets {
        fret := &song.Frets[i]
        if inpututil.IsKeyJustPressed(fret.Key) {
            fret.Press = time.Now()
        } else if inpututil.IsKeyJustReleased(fret.Key) {
            fret.Press = time.Time{}
        }
    }

    playGuitar := false
    stopGuitar := false
    changeGuitar := false

    delta := time.Since(song.StartTime)
    for i := range song.Frets {
        /*
        if i == 1 {
            break
        }
        */

        fret := &song.Frets[i]

        if fret.StartNote < len(fret.Notes) {
            for fret.StartNote < len(fret.Notes) && fret.Notes[fret.StartNote].End < delta + NoteThresholdLow {

                if fret.Notes[fret.StartNote].State == NoteStatePending {
                    fret.Notes[fret.StartNote].State = NoteStateMissed
                    song.NotesMissed += 1
                }

                fret.StartNote += 1
            }
        }

        pressed := inpututil.IsKeyJustPressed(fret.Key)

        // check if we are pressing the key for the current note
        for i := fret.StartNote; i < len(fret.Notes); i++ {
            note := &fret.Notes[i]
            noteDiff := note.Start - delta

            if noteDiff > NoteThresholdHigh {
                break
            }

            if note.State == NoteStatePending {
                if noteDiff < NoteThresholdLow {
                    note.State = NoteStateMissed
                    song.NotesMissed += 1
                    stopGuitar = true
                    changeGuitar = true
                } else if noteDiff >= NoteThresholdLow && noteDiff <= NoteThresholdHigh && pressed {

                    note.State = NoteStateHit
                    song.NotesHit += 1
                    playGuitar = true
                    changeGuitar = true

                    /*
                    keyDiff := delta - fret.Press.Sub(engine.StartTime)
                    log.Printf("Key diff: %v", keyDiff)
                    if keyDiff >= NoteThresholdLow && keyDiff <= NoteThresholdHigh {
                        note.State = NoteStateHit
                        engine.NotesHit += 1
                    } else {
                        note.State = NoteStateMissed
                        engine.NotesMissed += 1
                    }
                    */
                }
            }
        }

    }

    if changeGuitar {
        if playGuitar && !stopGuitar {
            if !song.Guitar.IsPlaying() {
                song.Guitar.SetPosition(delta)
                song.Guitar.Play()
            }
        } else {
            song.Guitar.Pause()
        }
    }

}

func MakeSong(audioContext *audio.Context, songDirectory string) (*Song, error) {
    song := Song{
        Frets: make([]Fret, 5),
    }

    song.Frets[0].Key = ebiten.Key1
    song.Frets[1].Key = ebiten.Key2
    song.Frets[2].Key = ebiten.Key3
    song.Frets[3].Key = ebiten.Key4
    song.Frets[4].Key = ebiten.Key5
    // engine.Frets[5].Key = ebiten.Key6

    difficulty := "easy"
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

    songPath := filepath.Join(songDirectory, "song.ogg")

    songPlayer, cleanup, err := loadOgg(audioContext, songPath)
    if err != nil {
        return nil, fmt.Errorf("Unable to create audio player for ogg file '%v': %v", songPath, err)
    }

    song.CleanupFuncs = append(song.CleanupFuncs, cleanup)

    guitarPath := filepath.Join(songDirectory, "guitar.ogg")
    guitarPlayer, cleanup, err := loadOgg(audioContext, guitarPath)
    if err != nil {
        return nil, fmt.Errorf("Unable to create audio player for ogg file '%v': %v", guitarPath, err)
    }

    song.CleanupFuncs = append(song.CleanupFuncs, cleanup)

    song.Song = songPlayer
    song.Guitar = guitarPlayer

    notesPath := filepath.Join(songDirectory, "notes.mid")

    reader := smflib.ReadTracks(notesPath, 2)
    if reader.Error() != nil {
        return nil, reader.Error()
    }
    reader.Do(func (event smflib.TrackEvent) {
        // log.Printf("Tick: %d, Microseconds: %v, Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.Message)
        var channel, key, velocity uint8
        if event.Message.GetNoteOn(&channel, &key, &velocity) {
            if int(key) >= low && int(key) <= high {
                // log.Printf("Tick: %d, Microseconds: %v, Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.Message)
                if velocity > 0 {
                    useFret := int(key) - low
                    fret := &song.Frets[useFret]
                    fret.Notes = append(fret.Notes, Note{
                        Start: time.Microsecond * time.Duration(event.AbsMicroSeconds),
                    })
                } else {
                    useFret := int(key) - low
                    fret := &song.Frets[useFret]
                    if len(fret.Notes) > 0 {
                        lastNote := &fret.Notes[len(fret.Notes)-1]
                        lastNote.End = time.Microsecond * time.Duration(event.AbsMicroSeconds)
                    }
                }
            }
        }
    })

    return &song, nil
}

type Engine struct {
    AudioContext *audio.Context
    CurrentSong *Song
}

func loadOgg(audioContext *audio.Context, songPath string) (*audio.Player, func(), error) {
    songDataReader, err := os.Open(songPath)
    if err != nil {
        return nil, nil, err
    }

    songReader, err := vorbis.DecodeWithSampleRate(audioContext.SampleRate(), songDataReader)
    if err != nil {
        return nil, nil, err
    }

    songPlayer, err := audioContext.NewPlayer(songReader)
    if err != nil {
        return nil, nil, err
    }

    return songPlayer, func(){ songDataReader.Close() }, nil
}

func MakeEngine(audioContext *audio.Context, songDirectory string) (*Engine, error) {
    engine := &Engine{
        AudioContext: audioContext,
    }

    song, err := MakeSong(audioContext, songDirectory)
    if err != nil {
        return nil, fmt.Errorf("Failed to make song: %v", err)
    }

    engine.CurrentSong = song

    return engine, nil
}

func (engine *Engine) Close() {
    if engine.CurrentSong != nil {
        engine.CurrentSong.Close()
    }
}

func (engine *Engine) TakeScreenshot() {
    output := ebiten.NewImage(ScreenWidth, ScreenHeight)
    output.Fill(color.NRGBA{R: 0, G: 0, B: 0, A: 255})
    engine.Draw(output)
    filename := fmt.Sprintf("rhythm-%s.png", time.Now().Format("2006-01-02-150405"))
    file, err := os.Create(filename)
    if err == nil {
        png.Encode(file, output)
        file.Close()
        log.Printf("Saved screenshot to %s", filename)
    }
}

func (engine *Engine) Update() error {

    keys := inpututil.AppendJustPressedKeys(nil)
    for _, key := range keys {
        switch key {
            case ebiten.KeyEscape, ebiten.KeyCapsLock:
                return ebiten.Termination
            case ebiten.KeyF1:
                engine.TakeScreenshot()
        }
    }

    engine.CurrentSong.Update()

    return nil
}

// vertical layout
func (engine *Engine) Draw(screen *ebiten.Image) {
    ebitenutil.DebugPrintAt(screen, fmt.Sprintf("FPS: %.2f", ebiten.ActualFPS()), 10, 10)

    playLine := ScreenHeight - 100

    white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
    grey := color.NRGBA{R: 100, G: 100, B: 100, A: 255}

    xStart := 50
    xWidth := 500

    vector.StrokeLine(screen, float32(xStart), float32(playLine), float32(xStart + xWidth), float32(playLine), 5, white, true)

    const noteSpeed = 5

    thresholdLow := int64(NoteThresholdLow / time.Millisecond) / noteSpeed
    thresholdHigh := int64(NoteThresholdHigh / time.Millisecond) / noteSpeed

    vector.FillRect(screen, float32(xStart), float32(playLine - int(thresholdHigh)), float32(xWidth), float32(int(thresholdHigh - thresholdLow)), color.NRGBA{R: 100, G: 100, B: 100, A: 100}, true)

    const noteSize = 30

    delta := time.Since(engine.CurrentSong.StartTime)
    for i := range engine.CurrentSong.Frets {
        fret := &engine.CurrentSong.Frets[i]

        xFret := 100 + i * 100

        vector.StrokeLine(screen, float32(xFret), 0, float32(xFret), float32(ScreenHeight), 3, white, true)

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
        vector.FillCircle(screen, float32(xFret), float32(playLine), noteSize, transparent, false)

        if !fret.Press.IsZero() {
            vector.StrokeCircle(screen, float32(xFret), float32(playLine), noteSize + 1, 2, white, false)
        }

        renderNote := func(note *Note) bool {
            start := -int64((note.Start - delta) / time.Millisecond) / noteSpeed + int64(playLine)
            end := -int64((note.End - delta) / time.Millisecond) / noteSpeed + int64(playLine)

            if -end > ScreenHeight + 100 {
                return false
            }

            if (start < ScreenHeight + 20 && start > -100) || (end < ScreenHeight + 20 && end > -100) {
                y := int(start)

                vector.FillCircle(screen, float32(xFret), float32(y), noteSize, fretColor, true)
                if note.State == NoteStateHit {
                    vector.StrokeCircle(screen, float32(xFret), float32(y), noteSize + 1, 2, white, false)
                } else if note.State == NoteStateMissed {
                    vector.StrokeCircle(screen, float32(xFret), float32(y), noteSize + 1, 2, grey, false)
                }

                if -(end - start) > 200 / noteSpeed {
                    thickness := 10

                    x1 := xFret - thickness / 2
                    y1 := end

                    vector.FillRect(screen, float32(x1), float32(y1), float32(thickness), float32(-(end - start)), fretColor, true)

                    /*
                    if note.State == NoteStateHit {
                        vector.StrokeRect(screen, float32(x), float32(yFret - thickness / 2), float32(end - start), float32(thickness), 2, white, false)
                    }
                    */
                }

                return true
            }

            return false
        }

        for i := fret.StartNote - 1; i >= 0 && i < len(fret.Notes); i-- {
            note := &fret.Notes[i]
            if !renderNote(note) {
                break
            }
        }

        for i := fret.StartNote; i < len(fret.Notes); i++ {
            note := &fret.Notes[i]
            if !renderNote(note) {
                break
            }
        }

    }
}

// horizontal layout
func (engine *Engine) Draw2(screen *ebiten.Image) {

    ebitenutil.DebugPrintAt(screen, fmt.Sprintf("FPS: %.2f", ebiten.ActualFPS()), 10, 10)

    playLine := 180

    white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

    vector.StrokeLine(screen, float32(playLine), 20, float32(playLine), 20 + 600, 5, color.RGBA{R: 255, G: 255, B: 255, A: 255}, true)

    const noteSpeed = 5

    thresholdLow := int64(NoteThresholdLow / time.Millisecond) / noteSpeed
    thresholdHigh := int64(NoteThresholdHigh / time.Millisecond) / noteSpeed

    vector.FillRect(screen, float32(playLine + int(thresholdLow)), 20, float32(int(thresholdHigh - thresholdLow)), 600, color.NRGBA{R: 100, G: 100, B: 100, A: 100}, true)

    const noteSize = 30

    delta := time.Since(engine.CurrentSong.StartTime)
    for i, fret := range engine.CurrentSong.Frets {

        yFret := 100 + i * 100

        vector.StrokeLine(screen, 0, float32(yFret), float32(ScreenWidth), float32(yFret), 3, color.RGBA{R: 200, G: 200, B: 200, A: 255}, true)

        if i == 0 {
            if fret.StartNote < len(fret.Notes) {
                currentNote := fret.Notes[fret.StartNote]
                diff := (currentNote.Start - delta).Milliseconds()
                ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%vms", diff), playLine - 80, yFret - 20)
            }
        }

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
        vector.FillCircle(screen, float32(playLine), float32(yFret), noteSize, transparent, false)

        if !fret.Press.IsZero() {
            vector.StrokeCircle(screen, float32(playLine), float32(yFret), noteSize + 1, 2, white, false)
        }

        renderNote := func(note *Note) bool {
            start := int64((note.Start - delta) / time.Millisecond) / noteSpeed + int64(playLine)
            end := int64((note.End - delta) / time.Millisecond) / noteSpeed + int64(playLine)

            if start > ScreenWidth + 100 {
                return false
            }

            if (start < ScreenWidth + 20 && start > -100) || (end < ScreenWidth + 20 && end > -100) {
                x := int(start)

                vector.FillCircle(screen, float32(x), float32(yFret), noteSize, fretColor, true)
                if note.State == NoteStateHit {
                    vector.StrokeCircle(screen, float32(x), float32(yFret), noteSize + 1, 2, white, false)
                }

                if end - start > 200 / noteSpeed {
                    thickness := 10
                    vector.FillRect(screen, float32(x), float32(yFret - thickness / 2), float32(end - start), float32(thickness), fretColor, true)

                    if note.State == NoteStateHit {
                        vector.StrokeRect(screen, float32(x), float32(yFret - thickness / 2), float32(end - start), float32(thickness), 2, white, false)
                    }
                }

                return true
            }

            return false
        }

        for i := fret.StartNote - 1; i >= 0 && i < len(fret.Notes); i-- {
            note := &fret.Notes[i]
            if !renderNote(note) {
                break
            }
        }

        for i := fret.StartNote; i < len(fret.Notes); i++ {
            note := &fret.Notes[i]
            if !renderNote(note) {
                break
            }
        }
    }

    ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Notes Hit: %d", engine.CurrentSong.NotesHit), 10, ScreenHeight - 40)
    ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Notes Missed: %d", engine.CurrentSong.NotesMissed), 10, ScreenHeight - 20)
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

    ebiten.SetTPS(120)
    ebiten.SetWindowSize(ScreenWidth, ScreenHeight)
    ebiten.SetWindowTitle("Rhythm Game")

    audioContext := audio.NewContext(44100)

    engine, err := MakeEngine(audioContext, "Queen - Killer Queen")

    if err != nil {
        log.Fatalf("Failed to make engine: %v", err)
    }
    defer engine.Close()

    err = ebiten.RunGame(engine)
    if err != nil && err != ebiten.Termination {
        log.Fatalf("Error running game: %v", err)
    }

    log.Printf("Bye")
}
