package main

import (
    "log"
    "time"
    "math"
    "math/rand/v2"
    "os"
    "io"
    "io/fs"
    "fmt"
    "bufio"
    "archive/zip"
    "image/color"
    "image/png"
    "image/jpeg"
    "path/filepath"
    "bytes"
    "sync"
    "strings"
    "errors"

    "github.com/kazzmir/rhythm/lib/coroutine"
    "github.com/kazzmir/rhythm/lib/colorconv"
    "github.com/kazzmir/rhythm/data"

    smflib "gitlab.com/gomidi/midi/v2/smf"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/audio"
    "github.com/hajimehoshi/ebiten/v2/audio/vorbis"
    "github.com/hajimehoshi/ebiten/v2/audio/mp3"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
    "github.com/hajimehoshi/ebiten/v2/vector"
    // "github.com/hajimehoshi/ebiten/v2/ebitenutil"
    "github.com/hajimehoshi/ebiten/v2/text/v2"

    "github.com/solarlune/tetra3d"
)

const ScreenWidth = 1400
const ScreenHeight = 1000

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
    Sustain bool
}

func (note *Note) HasSustain() bool {
    return note.End - note.Start > time.Millisecond * 200
}

type Fret struct {
    InUse bool
    Notes []Note

    // the index of the next note to be hit
    StartNote int

    // time when this key was pressed (or zero if not pressed)
    Press time.Time
    // Key ebiten.Key
    InputAction InputAction
}

type InputAction int
const (
    InputActionNone InputAction = iota
    InputActionGreen
    InputActionRed
    InputActionYellow
    InputActionBlue
    InputActionOrange
    InputActionStrumUp
    InputActionStrumDown
)

type Input struct {
    HasGamepad bool
    GamepadID ebiten.GamepadID
    GamepadButtons map[InputAction]ebiten.GamepadButton
    KeyboardButtons map[InputAction]ebiten.Key
}

func MakeDefaultInput() *Input {
    return &Input{
        KeyboardButtons: map[InputAction]ebiten.Key{
            InputActionGreen: ebiten.Key1,
            InputActionRed: ebiten.Key2,
            InputActionYellow: ebiten.Key3,
            InputActionBlue: ebiten.Key4,
            InputActionOrange: ebiten.Key5,
            InputActionStrumUp: ebiten.KeyUp,
            InputActionStrumDown: ebiten.KeySpace,
        },
    }
}

type SongInfo struct {
    Artist string
    Name string
    Album string
    Genre string
    Year string
    SongLength time.Duration
}

type FlameMaker interface {
    MakeFlame(fret int)
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
    SongLength time.Duration

    Counter uint64

    Score int

    SongInfo SongInfo
}

func (song *Song) Finished() bool {
    delta := time.Since(song.StartTime)
    return delta >= song.SongLength + time.Second * 2
}

// total notes seen so far
func (song *Song) TotalNotes() int {
    return song.NotesHit + song.NotesMissed
}

func (song *Song) Close() {
    song.Song.Pause()
    song.Guitar.Pause()

    for _, cleanup := range song.CleanupFuncs {
        cleanup()
    }
}

func (song *Song) Update(input *Input, flameMaker FlameMaker) {
    song.Counter += 1

    song.DoSong.Do(func(){
        song.Song.Play()
        song.Guitar.Play()
    })

    if song.StartTime.IsZero() {
        song.StartTime = time.Now()
    }

    /*
    for id := range gamepadIds {
        maxButton := ebiten.GamepadButton(ebiten.GamepadButtonCount(id))
        for button := ebiten.GamepadButton(0); button < maxButton; button++ {
            if inpututil.IsGamepadButtonJustPressed(id, button) {
                log.Printf("Pressed button %v on gamepad %v", button, id)
            }
            if inpututil.IsGamepadButtonJustReleased(id, button) {
                log.Printf("Released button %v on gamepad %v", button, id)
            }
        }
    }
    */

    for i := range song.Frets {
        fret := &song.Frets[i]
        key := input.KeyboardButtons[fret.InputAction]
        if inpututil.IsKeyJustPressed(key) {
            fret.Press = time.Now()
        } else if inpututil.IsKeyJustReleased(key) {
            fret.Press = time.Time{}
        }

        if input.HasGamepad {
            button := input.GamepadButtons[fret.InputAction]
            if inpututil.IsGamepadButtonJustPressed(input.GamepadID, button) {
                fret.Press = time.Now()
            } else if inpututil.IsGamepadButtonJustReleased(input.GamepadID, button) {
                fret.Press = time.Time{}
            }
        }
    }

    playGuitar := false
    stopGuitar := false
    changeGuitar := false

    // when true, we don't need to strum
    allTapsMode := false

    forceMiss := false

    var notesHit []*Note

    strummed := inpututil.IsKeyJustPressed(input.KeyboardButtons[InputActionStrumDown])
    if input.HasGamepad {
        button := input.GamepadButtons[InputActionStrumDown]
        strummed = strummed || inpututil.IsGamepadButtonJustPressed(input.GamepadID, button)
    }

    delta := time.Since(song.StartTime)
    for fretIndex := range song.Frets {
        /*
        if i == 1 {
            break
        }
        */

        fret := &song.Frets[fretIndex]

        if fret.StartNote < len(fret.Notes) {
            for fret.StartNote < len(fret.Notes) && fret.Notes[fret.StartNote].End < delta + NoteThresholdLow {

                if fret.Notes[fret.StartNote].State == NoteStatePending {
                    fret.Notes[fret.StartNote].State = NoteStateMissed
                    song.NotesMissed += 1
                }

                fret.StartNote += 1
            }
        }

        pressed := false

        if allTapsMode {
            key := input.KeyboardButtons[fret.InputAction]
            pressed = inpututil.IsKeyJustPressed(key)
            if input.HasGamepad {
                button := input.GamepadButtons[fret.InputAction]
                if inpututil.IsGamepadButtonJustPressed(input.GamepadID, button) {
                    pressed = true
                }
            }
        } else {
            pressed = !fret.Press.IsZero() && strummed
        }

        needKey := false

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
                } else if noteDiff >= NoteThresholdLow && noteDiff <= NoteThresholdHigh {
                    // user should have pressed the key here
                    needKey = true

                    if pressed {
                        notesHit = append(notesHit, note)
                        flameMaker.MakeFlame(fretIndex)
                    }

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
            } else if note.State == NoteStateHit && note.Sustain {
                // determine if the note has a sustained part and the keys are still held
                if note.End > delta && note.End - note.Start > time.Millisecond * 200 {
                    if fret.Press.IsZero() {
                        note.Sustain = false
                    } else {
                        song.Score += 1

                        if song.Counter % 5 == 0 {
                            flameMaker.MakeFlame(fretIndex)
                        }
                    }
                }
            }
        }

        if !needKey && pressed {
            forceMiss = true
        }
    }

    if forceMiss {
        for _, note := range notesHit {
            note.State = NoteStateMissed
            note.Sustain = false
            song.NotesMissed += 1
            playGuitar = false
            changeGuitar = true
        }
    } else {
        for _, note := range notesHit {
            note.State = NoteStateHit
            note.Sustain = true
            song.NotesHit += 1
            playGuitar = true
            changeGuitar = true
            song.Score += 5
        }
    }

    if changeGuitar {
        if playGuitar && !stopGuitar {

            song.Guitar.SetVolume(1.0)

            /*
            if !song.Guitar.IsPlaying() {
                err := song.Guitar.SetPosition(delta)
                if err != nil {
                    log.Printf("Failed to set guitar position: %v", err)
                }
                song.Guitar.Play()
            }
            */
        } else {
            song.Guitar.SetVolume(0.3)
            // song.Guitar.Pause()
        }
    }
}

// returns the index of the guitar track, or -1 if not found
func findGuitarTrack(smf *smflib.SMF) int {
    for i, track := range smf.Tracks {
        if len(track) > 0 {
            event := track[0]
            var trackName string
            if event.Message.GetMetaTrackName(&trackName) {
                if strings.Contains(strings.ToLower(trackName), "guitar") {
                    return i
                }
            }
        }
    }

    return -1
}

func isZip(path string) bool {
    file, err := os.Open(path)
    if err != nil {
        return false
    }

    defer file.Close()

    buffer := make([]byte, 4)
    _, err = file.Read(buffer)
    if err != nil {
        return false
    }

    return string(buffer) == "PK\x03\x04"
}

func getFileSize(file *os.File) int64 {
    info, err := file.Stat()
    if err != nil {
        return 0
    }
    return info.Size()
}

func findFile(basefs fs.FS, name string) (fs.File, error) {
    // try direct open first
    file, err := basefs.Open(name)
    if err == nil {
        return file, nil
    }

    // walk the FS to find the file
    var foundFile fs.File
    err = fs.WalkDir(basefs, ".", func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        if !d.IsDir() && strings.ToLower(filepath.Base(path)) == strings.ToLower(name) {
            file, err := basefs.Open(path)
            if err != nil {
                return err
            }
            foundFile = file
            return fs.SkipDir
        }

        return nil
    })

    if err != nil {
        return nil, err
    }

    if foundFile == nil {
        return nil, fmt.Errorf("file '%v' not found", name)
    }

    return foundFile, nil
}

func MakeSong(audioContext *audio.Context, songDirectory string, difficulty string) (*Song, error) {
    song := Song{
        Frets: make([]Fret, 5),
    }

    song.Frets[0].InputAction = InputActionGreen
    song.Frets[1].InputAction = InputActionRed
    song.Frets[2].InputAction = InputActionYellow
    song.Frets[3].InputAction = InputActionBlue
    song.Frets[4].InputAction = InputActionOrange
    // song.Frets[5].Key = ebiten.Key6

    var basefs fs.FS

    if isZip(songDirectory) {
        zipFile, err := os.Open(songDirectory)
        if err != nil {
            return nil, fmt.Errorf("Unable to open song zip file '%v': %v", songDirectory, err)
        }

        defer zipFile.Close()

        zipper, err := zip.NewReader(zipFile, getFileSize(zipFile))
        if err != nil {
            return nil, fmt.Errorf("Unable to read song zip file '%v': %v", songDirectory, err)
        }

        basefs = zipper
    } else {
        basefs = os.DirFS(songDirectory)
    }

    var songLength time.Duration
    var songPlayer *audio.Player

    songFile, err := findFile(basefs, "song.ogg")
    if err == nil {
        defer songFile.Close()

        var cleanup func()

        songPlayer, songLength, cleanup, err = loadOgg(audioContext, songFile, "song.ogg")
        if err != nil {
            return nil, fmt.Errorf("Unable to create audio player for ogg file '%v': %v", "song.ogg", err)
        }
        song.CleanupFuncs = append(song.CleanupFuncs, cleanup)
    } else {
        songFile, err = findFile(basefs, "song.mp3")
        if err == nil {
            defer songFile.Close()

            var cleanup func()
            songPlayer, songLength, cleanup, err = loadMp3(audioContext, songFile, "song.mp3")
            if err != nil {
                return nil, fmt.Errorf("Unable to create audio player for ogg file '%v': %v", "song.mp3", err)
            }

            song.CleanupFuncs = append(song.CleanupFuncs, cleanup)
        } else {
            return nil, fmt.Errorf("Unable to find song.ogg or song.mp3 in song directory '%v': %v", songDirectory, err)
        }
    }

    var guitarPlayer *audio.Player

    guitarFile, err := findFile(basefs, "guitar.ogg")
    if err == nil {
        defer guitarFile.Close()
        var cleanup func()
        guitarPlayer, _, cleanup, err = loadOgg(audioContext, guitarFile, "guitar.ogg")
        if err != nil {
            return nil, fmt.Errorf("Unable to create audio player for ogg file '%v': %v", "guitar.ogg", err)
        }
        song.CleanupFuncs = append(song.CleanupFuncs, cleanup)
    } else {
        guitarFile, err = findFile(basefs, "guitar.mp3")
        if err == nil {
            defer guitarFile.Close()
            var cleanup func()
            guitarPlayer, _, cleanup, err = loadMp3(audioContext, guitarFile, "guitar.mp3")
            if err != nil {
                return nil, fmt.Errorf("Unable to create audio player for ogg file '%v': %v", "guitar.mp3", err)
            }
            song.CleanupFuncs = append(song.CleanupFuncs, cleanup)
        } else {
            return nil, fmt.Errorf("Unable to find guitar.ogg in song directory '%v': %v", songDirectory, err)
        }
    }

    song.SongLength = songLength
    song.Song = songPlayer
    song.Guitar = guitarPlayer

    // notesPath := filepath.Join(songDirectory, "notes.mid")

    notesFile, err := findFile(basefs, "notes.mid")
    if err != nil {
        return nil, fmt.Errorf("Unable to open MIDI file '%v': %v", "notes.mid", err)
    }
    defer notesFile.Close()

    notesData, err := io.ReadAll(bufio.NewReader(notesFile))
    if err != nil {
        return nil, fmt.Errorf("Unable to read MIDI file '%v': %v", "notes.mid", err)
    }

    err = song.ReadNotes(notesData, difficulty, songLength)
    if err != nil {
        return nil, err
    }

    iniFile, err := findFile(basefs, "song.ini")
    if err == nil {
        defer iniFile.Close()
        song.SongInfo = loadSongInfo(iniFile)
        log.Printf("Loaded song info: %+v", song.SongInfo)
    }

    return &song, nil
}

// load song info from song.ini file
func loadSongInfo(file fs.File) SongInfo {
    var out SongInfo

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())

        if line == "[song]" {
            continue
        }

        name, value, ok := strings.Cut(line, "=")
        if ok {
            name = strings.ToLower(strings.TrimSpace(name))
            value = strings.TrimSpace(value)

            switch name {
                case "artist": out.Artist = value
                case "name": out.Name = value
                case "album": out.Album = value
                case "genre": out.Genre = value
                case "year": out.Year = value
            }
        }
    }

    return out
}

// notesData is assumed to be the contents of a MIDI file
func (song *Song) ReadNotes(notesData []byte, difficulty string, songLength time.Duration) error {

    // FIXME: dire straits sultans of swing uses keys higher than the normal range

    var low, high int
    switch difficulty {
        case "easy":
            low = 60
            high = 64
        case "medium":
            low = 72
            high = 76
        case "hard":
            high = 84
            low = 88
        case "expert":
            high = 96
            low = 100
    }

    smf, err := smflib.ReadFrom(bytes.NewReader(notesData))
    if err != nil {
        return fmt.Errorf("Unable to read MIDI file '%v': %v", "notes.mid", err)
    }

    guitarTrack := findGuitarTrack(smf)

    if guitarTrack == -1 {
        return fmt.Errorf("Unable to find guitar track in MIDI file '%v'", "notes.mid")
    }

    log.Printf("Using guitar track %d for notes", guitarTrack)

    reader := smflib.ReadTracksFrom(bytes.NewReader(notesData), guitarTrack)
    if reader.Error() != nil {
        return reader.Error()
    }
    reader.Do(func (event smflib.TrackEvent) {
        // log.Printf("Tick: %d, Microseconds: %v, Track %v Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.TrackNo, event.Message)
        var channel, key, velocity uint8
        if event.Message.GetNoteOn(&channel, &key, &velocity) {
            if int(key) >= low && int(key) <= high {
                // log.Printf("Tick: %d, Microseconds: %v, Event: %v", event.AbsTicks, event.AbsMicroSeconds, event.Message)
                if velocity > 0 {
                    useFret := int(key) - low
                    if useFret >= 0 && useFret < len(song.Frets) {
                        fret := &song.Frets[useFret]
                        fret.Notes = append(fret.Notes, Note{
                            Start: time.Microsecond * time.Duration(event.AbsMicroSeconds),
                            End: songLength,
                        })
                    } else {
                        log.Printf("Warning: Fret %d out of range for key %d", useFret, key)
                    }
                } else {
                    useFret := int(key) - low
                    if useFret >= 0 && useFret < len(song.Frets) {
                        fret := &song.Frets[useFret]
                        if len(fret.Notes) > 0 {
                            lastNote := &fret.Notes[len(fret.Notes)-1]
                            lastNote.End = time.Microsecond * time.Duration(event.AbsMicroSeconds)
                        }
                    } else {
                        log.Printf("Warning: Fret %d out of range for key %d", useFret, key)
                    }
                }
            }
        }
        // some songs use NoteOff and others use NoteOn with a velocity of 0
        if event.Message.GetNoteOff(&channel, &key, &velocity) {
            if int(key) >= low && int(key) <= high {
                useFret := int(key) - low
                if useFret >= 0 && useFret < len(song.Frets) {
                    fret := &song.Frets[useFret]
                    if len(fret.Notes) > 0 {
                        lastNote := &fret.Notes[len(fret.Notes)-1]
                        lastNote.End = time.Microsecond * time.Duration(event.AbsMicroSeconds)
                    }
                } else {
                    log.Printf("Warning: Fret %d out of range for key %d", useFret, key)
                }
            }
        }
    })

    return nil
}

type Engine struct {
    AudioContext *audio.Context
    // CurrentSong *Song
    Font *text.GoTextFaceSource

    Drawers []func(screen *ebiten.Image)

    Input *Input

    Coroutine *coroutine.Coroutine

    GamepadIds map[ebiten.GamepadID]struct{}

    // GuitarButtonMesh *tetra3d.Mesh
}

func (engine *Engine) PushDrawer(drawer func(screen *ebiten.Image)) {
    engine.Drawers = append(engine.Drawers, drawer)
}

func (engine *Engine) PopDrawer() {
    if len(engine.Drawers) > 0 {
        engine.Drawers = engine.Drawers[:len(engine.Drawers)-1]
    }
}

func loadMp3(audioContext *audio.Context, file fs.File, name string) (*audio.Player, time.Duration, func(), error) {
    allData, err := io.ReadAll(bufio.NewReader(file))
    if err != nil {
        return nil, 0, nil, err
    }

    songReader, err := mp3.DecodeWithSampleRate(audioContext.SampleRate(), bytes.NewReader(allData))
    if err != nil {
        return nil, 0, nil, err
    }

    // divide by 2 for 16-bit samples, divide by 2 for stereo
    length := songReader.Length() / 2 / 2 / int64(songReader.SampleRate())
    // log.Printf("OGG file '%s' rate %v bytes %v length: %v", name, songReader.SampleRate(), songReader.Length(), time.Duration(length) * time.Second)

    songPlayer, err := audioContext.NewPlayer(songReader)
    if err != nil {
        return nil, 0, nil, err
    }

    log.Printf("Loaded MP3 file '%s' length %d", name, len(allData))

    return songPlayer, time.Duration(length) * time.Second, func(){}, nil
}

func loadOgg(audioContext *audio.Context, file fs.File, name string) (*audio.Player, time.Duration, func(), error) {
    allData, err := io.ReadAll(bufio.NewReader(file))
    if err != nil {
        return nil, 0, nil, err
    }

    songReader, err := vorbis.DecodeWithSampleRate(audioContext.SampleRate(), bytes.NewReader(allData))
    if err != nil {
        return nil, 0, nil, err
    }

    // divide by 2 for 16-bit samples, divide by 2 for stereo
    length := songReader.Length() / 2 / 2 / int64(songReader.SampleRate())
    // log.Printf("OGG file '%s' rate %v bytes %v length: %v", name, songReader.SampleRate(), songReader.Length(), time.Duration(length) * time.Second)

    songPlayer, err := audioContext.NewPlayer(songReader)
    if err != nil {
        return nil, 0, nil, err
    }

    log.Printf("Loaded OGG file '%s' length %d", name, len(allData))

    return songPlayer, time.Duration(length) * time.Second, func(){}, nil
}

func MakeEngine(audioContext *audio.Context, songDirectory string) (*Engine, error) {
    font, err := LoadFont()
    if err != nil {
        return nil, fmt.Errorf("Failed to load font: %v", err)
    }

    var engine *Engine

    engine = &Engine{
        AudioContext: audioContext,
        Font: font,
        GamepadIds: make(map[ebiten.GamepadID]struct{}),
        Input: MakeDefaultInput(),
        Coroutine: coroutine.MakeCoroutine(func(yield coroutine.YieldFunc) error {
            if songDirectory != "" {
                err := playSong(yield, engine, songDirectory, DefaultSongSettings())
                return err
            }

            return mainMenu(engine, yield)
        }),
        // GuitarButtonMesh: tetra3d.NewCylinderMesh(2, 40, 50, false),
    }

    /*
    song, err := MakeSong(audioContext, songDirectory)
    if err != nil {
        return nil, fmt.Errorf("Failed to make song: %v", err)
    }

    engine.CurrentSong = song
    */

    return engine, nil
}

func (engine *Engine) Close() {
    /*
    if engine.CurrentSong != nil {
        engine.CurrentSong.Close()
    }
    */
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
            /*
            case ebiten.KeyEscape, ebiten.KeyCapsLock:
                return ebiten.Termination
                */
            case ebiten.KeyF1:
                engine.TakeScreenshot()
        }
    }

    newGamepadIDs := inpututil.AppendJustConnectedGamepadIDs(nil)
    for _, id := range newGamepadIDs {
        log.Printf("Gamepad connected: %v '%s'", id, ebiten.GamepadName(id))
        engine.GamepadIds[id] = struct{}{}

        if !engine.Input.HasGamepad {
            engine.Input.HasGamepad = true
            engine.Input.GamepadID = id
            // works for PDP Rock Band 4 Jaguar
            engine.Input.GamepadButtons = map[InputAction]ebiten.GamepadButton{
                InputActionGreen: ebiten.GamepadButton3,
                InputActionRed: ebiten.GamepadButton4,
                InputActionYellow: ebiten.GamepadButton5,
                InputActionBlue: ebiten.GamepadButton6,
                InputActionOrange: ebiten.GamepadButton7,
                InputActionStrumUp: ebiten.GamepadButton13,
                InputActionStrumDown: ebiten.GamepadButton15,
            }
        }
    }
    for id := range engine.GamepadIds {
        if inpututil.IsGamepadJustDisconnected(id) {
            delete(engine.GamepadIds, id)
        }
    }

    if ebiten.IsWindowBeingClosed() {
        engine.Coroutine.Stop()
    }

    err := engine.Coroutine.Run()
    if err != nil {
        if errors.Is(err, coroutine.CoroutineFinished) || errors.Is(err, coroutine.CoroutineCancelled) {
            return ebiten.Termination
        }

        return err
    }

    // engine.CurrentSong.Update()

    return nil
}

type SongSettings struct {
    Difficulty string
}

func DefaultSongSettings() SongSettings {
    return SongSettings{
        Difficulty: "medium",
    }
}

func is_convex(a, b, c tetra3d.VertexInfo, normal tetra3d.Vector3) bool {
    a_vec := tetra3d.NewVector3(a.X, a.Y, a.Z)
    b_vec := tetra3d.NewVector3(b.X, b.Y, b.Z)
    c_vec := tetra3d.NewVector3(c.X, c.Y, c.Z)

    ab := a_vec.Sub(b_vec)
    cb := c_vec.Sub(b_vec)

    cross := ab.Cross(cb)
    return cross.Dot(normal) > 0
}

func pointInTriangle(p, a, b, c tetra3d.VertexInfo, normal tetra3d.Vector3) bool {
    p_vec := tetra3d.NewVector3(p.X, p.Y, p.Z)
    a_vec := tetra3d.NewVector3(a.X, a.Y, a.Z)
    b_vec := tetra3d.NewVector3(b.X, b.Y, b.Z)
    c_vec := tetra3d.NewVector3(c.X, c.Y, c.Z)
    /*
    ab := b_vec.Sub(a_vec)
    ac := c_vec.Sub(a_vec)
    ap := p_vec.Sub(a_vec)
    d1 := ab.Dot(ap)
    d2 := ac.Dot(ap)
    if d1 < 0 || d2 < 0 {
        return false
    }
    d3 := ab.Dot(ab)
    d4 := ac.Dot(ac)
    if d1 > d3 || d2 > d4 {
        return false
    }
    vc := d3*d2 - d1*d4
    return vc >= 0
    */

    e0 := b_vec.Sub(a_vec).Cross(p_vec.Sub(a_vec)).Dot(normal)
    e1 := c_vec.Sub(b_vec).Cross(p_vec.Sub(b_vec)).Dot(normal)
    e2 := a_vec.Sub(c_vec).Cross(p_vec.Sub(c_vec)).Dot(normal)

    if e0 >= 0 && e1 >= 0 && e2 >= 0 {
        return true
    }

    return false
}

func no_vertex_inside(aIndex, bIndex, cIndex int, vertIndices []int, vertices []tetra3d.VertexInfo, normal tetra3d.Vector3) bool {
    for _, vi := range vertIndices {
        if vi == aIndex || vi == bIndex || vi == cIndex {
            continue
        }

        if pointInTriangle(vertices[vi], vertices[aIndex], vertices[bIndex], vertices[cIndex], normal) {
            return false
        }
    }

    return true
}

// implement the ear clipping algorithm for simple convex polygons. vertices must be in counter-clockwise order
func tesselate2(vertices []tetra3d.VertexInfo, vertIndices []int) []int {
    var out []int

    normal := tetra3d.NewVector3(0, 1, 0)

    for len(vertIndices) > 3 {
        fail := true
        for i := range vertIndices {
            prevIndex := (i - 1 + len(vertIndices)) % len(vertIndices)
            nextIndex := (i + 1) % len(vertIndices)
            a := vertIndices[prevIndex]
            b := vertIndices[i]
            c := vertIndices[nextIndex]

            if is_convex(vertices[a], vertices[b], vertices[c], normal) && no_vertex_inside(a, b, c, vertIndices, vertices, normal) {
                log.Printf("Clipping ear: %d, %d, %d", a, b, c)
                out = append(out, a, b, c)

                before := vertIndices[:i]
                after := vertIndices[i+1:]
                vertIndices = append(before, after...)
                fail = false
                break
            }
        }

        if fail {
            log.Printf("Tesselation failed; remaining vertices: %v", vertIndices)
            break
        }
    }

    return append(out, vertIndices...)
}

func tesselate(vertices []tetra3d.VertexInfo, vertIndices []int) []int {
    var out []int

    for len(vertIndices) > 3 {
        // remove index 1
        out = append(out, vertIndices[0], vertIndices[1], vertIndices[2])
        before := vertIndices[:1]
        after := vertIndices[2:]
        vertIndices = append(before, after...)
    }

    return append(out, vertIndices...)
}

// NewCylinderMesh creates a new cylinder Mesh and gives it a new material (suitably named "Cylinder").
// sideCount is how many sides the cylinder should have, while radius is the radius of the cylinder in world units.
// if createCaps is true, then the cylinder will have triangle caps.
func NewCylinderMesh(sideCount int, radius, height float32) *tetra3d.Mesh {

	if sideCount < 3 {
		sideCount = 3
	}

	mesh := tetra3d.NewMesh("Cylinder")

	verts := []tetra3d.VertexInfo{}

    var topVerts []int

	for i := 0; i < sideCount; i++ {

		pos := tetra3d.NewVector3(radius, height/2, 0)
		pos = pos.Rotate(0, 1, 0, float32(i)/float32(sideCount)*math.Pi*2)
        topVerts = append(topVerts, len(verts))
		verts = append(verts, tetra3d.NewVertex(pos.X, pos.Y, pos.Z, 0, 0))

	}

    var bottomVerts []int
	for i := 0; i < sideCount; i++ {

		pos := tetra3d.NewVector3(radius, -height/2, 0)
		pos = pos.Rotate(0, 1, 0, float32(i)/float32(sideCount)*math.Pi*2)
        bottomVerts = append(bottomVerts, len(verts))
		verts = append(verts, tetra3d.NewVertex(pos.X, pos.Y, pos.Z, 0, 0))
	}

    /*
	if createCaps {
		verts = append(verts, NewVertex(0, height/2, 0, 0, 0))
		verts = append(verts, NewVertex(0, -height/2, 0, 0, 0))
	}
    */

	mesh.AddVertices(verts...)

	indices := []int{}
	for i := 0; i < sideCount; i++ {
		if i < sideCount-1 {
			indices = append(indices, i, i+sideCount, i+1)
			indices = append(indices, i+1, i+sideCount, i+sideCount+1)
		} else {
			indices = append(indices, i, i+sideCount, 0)
			indices = append(indices, 0, i+sideCount, sideCount)
		}
	}

    // indices = append(indices, tesselate(verts, topVerts)...)

    /*
    indices = append(indices, topVerts...)
    indices = append(indices, bottomVerts...)
    */

    /*
	if createCaps {
		for i := 0; i < sideCount; i++ {
			topCenter := len(verts) - 2
			bottomCenter := len(verts) - 1
			if i < sideCount-1 {
				indices = append(indices, i, i+1, topCenter)
				indices = append(indices, i+sideCount+1, i+sideCount, bottomCenter)
			} else {
				indices = append(indices, i, 0, topCenter)
				indices = append(indices, sideCount, i+sideCount, bottomCenter)
			}
		}
	}
    */

	mesh.AddMeshPart(tetra3d.NewMaterial("Cylinder"), indices...)
    mesh.AddMeshPart(tetra3d.NewMaterial("CylinderTop"), tesselate(verts, topVerts)...)
    // mesh.AddMeshPart(tetra3d.NewMaterial("CylinderBottom"), tesselate(verts, bottomVerts)...)

	mesh.UpdateBounds()
	mesh.AutoNormal()

	return mesh
}

func make3dRectangle(width, height, depth float32, color tetra3d.Color) *tetra3d.Mesh {
	mesh := tetra3d.NewMesh("3dRectangle")

    x0 := -width / 2
    x1 := width / 2
    y0 := -height / 2
    y1 := height / 2
    z0 := float32(0)
    z1 := -depth

    var verts []tetra3d.VertexInfo

    for _, x := range []float32{x0, x1} {
        for _, y := range []float32{y0, y1} {
            for _, z := range []float32{z0, z1} {
                vertex := tetra3d.NewVertex(x, y, z, 0, 0)
                verts = append(verts, vertex)
            }
        }
    }

    frontPlane := []int{0, 4, 6, 2}
    rightPlane := []int{4, 6, 7, 5}
    leftPlane := []int{0, 1, 3, 2}
    backPlane := []int{1, 5, 7, 3}
    topPlane := []int{2, 6, 7, 3}
    bottomPlane := []int{0, 4, 5, 1}

    verts[frontPlane[0]].U = 0
    verts[frontPlane[0]].V = 0
    verts[frontPlane[1]].U = 1
    verts[frontPlane[1]].V = 0
    verts[frontPlane[2]].U = 1
    verts[frontPlane[2]].V = 1
    verts[frontPlane[3]].U = 0
    verts[frontPlane[3]].V = 1

    mesh.AddVertices(verts...)

    /*
    verts := []tetra3d.VertexInfo{
        tetra3d.NewVertex(x0, y0, z0, 0, 0), // 0
        tetra3d.NewVertex(x1, y0, z0, 0, 0), // 0
        tetra3d.NewVertex(x1, y1, z0, 0, 0), // 0
        tetra3d.NewVertex(x0, y1, z0, 0, 0), // 0

        tetra3d.NewVertex(x0, y1, z0, 0, 0), // 0
    }
    */

    _ = frontPlane
    _ = rightPlane
    _ = leftPlane
    _ = backPlane
    _ = topPlane
    _ = bottomPlane

    mesh.AddMeshPart(tetra3d.NewMaterial("Front"), tesselate(verts, frontPlane)...)
    // mesh.AddMeshPart(tetra3d.NewMaterial("Right"), tesselate(verts, rightPlane)...)
    // mesh.AddMeshPart(tetra3d.NewMaterial("Left"), tesselate(verts, leftPlane)...)
    mesh.AddMeshPart(tetra3d.NewMaterial("Back"), tesselate(verts, backPlane)...)
    mesh.AddMeshPart(tetra3d.NewMaterial("Top"), tesselate(verts, topPlane)...)
    mesh.AddMeshPart(tetra3d.NewMaterial("Bottom"), tesselate(verts, bottomPlane)...)

    for _, part := range mesh.MeshParts {
        name := part.Material.Name()
        tetra3d.NewVertexSelection().SelectMeshPartByName(mesh, name).SetColor(1, color)
    }

    // tetra3d.NewVertexSelection().SelectIndices(mesh, frontPlane[0], frontPlane[1]).SetColor(1, tetra3d.NewColor(0.1, 0.1, 0.1, 1))

    mesh.SetActiveColorChannel(1)

	mesh.UpdateBounds()
	mesh.AutoNormal()

    return mesh
}

type FlameManager struct {
    Images []*ebiten.Image
    Flames []*Flame
}

func NewFlameManager() *FlameManager {
    files, err := data.FlameFS.ReadDir("flame")
    if err != nil {
        log.Printf("Unable to read flame FS: %v", err)
        return nil
    }

    var images []*ebiten.Image

    for _, path := range files {
        if path.IsDir() {
            continue
        }
        img, err := func() (*ebiten.Image, error) {
            file, err := data.FlameFS.Open(filepath.Join("flame", path.Name()))
            if err != nil {
                return nil, err
            }
            defer file.Close()
            return loadPng(bufio.NewReader(file))
        }()

        if err != nil {
            log.Printf("Unable to load flame image '%v': %v", path.Name(), err)
        }

        images = append(images, img)
    }

    return &FlameManager{
        Images: images,
    }
}

func (manager *FlameManager) MakeFlame(fret int) {
    manager.Flames = append(manager.Flames, &Flame{
        Images: manager.Images,
        Life: 0,
        Fret: fret,
    })
}

func (manager *FlameManager) Update() {
    var flames []*Flame

    for _, flame := range manager.Flames {
        flame.Life += 1
        if flame.Image() != nil {
            flames = append(flames, flame)
        }
    }

    manager.Flames = flames
}

type Flame struct {
    Images []*ebiten.Image
    Life int
    Fret int
}

func (flame *Flame) Image() *ebiten.Image {
    index := flame.Life / 2
    if index >= len(flame.Images) {
        return nil
    }

    return flame.Images[index]
}

type ParticleManager struct {
    ParticleMesh *tetra3d.Mesh
    Particles []*Particle
    Scene *tetra3d.Scene
}

func NewParticleManager(scene *tetra3d.Scene) *ParticleManager {
    particleMesh := tetra3d.NewIcosphereMesh(1)

    return &ParticleManager{
        ParticleMesh: particleMesh,
        Scene: scene,
    }
}

func (manager *ParticleManager) MakeFlame(fret int) {

    newParticles := rand.N(4) + 6

    var color tetra3d.Color
    /*
    switch fret {
        case 0: color = tetra3d.NewColor(1, 0, 0, 1)
        case 1: color = tetra3d.NewColor(0, 1, 0, 1)
        case 2: color = tetra3d.NewColor(1, 1, 0, 1)
        case 3: color = tetra3d.NewColor(0, 0, 1, 1)
        case 4: color = tetra3d.NewColor(1, 0.5, 0, 1)
        default: color = tetra3d.NewColor(1, 1, 1, 1)
    }
    */
    color = tetra3d.NewColor(248.0/255.0, 134.0/255.0, 69.0/255.0, 1)

    for range newParticles {
        model := tetra3d.NewModel("Particle", manager.ParticleMesh)
        model.Color = color
        model.SetWorldPosition(float32((fret - 2) * 10), 0, 0)

        manager.Scene.Root.AddChildren(model)

        manager.Particles = append(manager.Particles, &Particle{
            Model: model,
            Life: 0,
            // Movement: tetra3d.NewVector3((rand.Float32() - 0.5), 0.1 + rand.Float32(), (rand.Float32() - 0.5)),
            Movement: tetra3d.NewVector3((rand.Float32() - 0.5) / 3, 0.1 + rand.Float32() / 2, (rand.Float32() - 0.5) / 3),
        })
    }
}

func (manager *ParticleManager) Update() {
    var particles []*Particle

    for _, particle := range manager.Particles {
        particle.Life += 1
        if particle.Life < 60 {
            particle.Model.Move(particle.Movement.X, particle.Movement.Y, particle.Movement.Z)

            particle.Model.Color = particle.Model.Color.MultiplyRGBA(1, 1, 1, 0.98)

            scale := particle.Model.LocalScale()
            particle.Model.SetLocalScale(scale.X * 0.98, scale.Y * 0.98, scale.Z * 0.98)

            // particle.Model.Grow(-0.98, -0.98, -0.98)
            // gravity
            particle.Movement.Y -= 0.01
            particles = append(particles, particle)
        } else {
            manager.Scene.Root.RemoveChildren(particle.Model)
        }
    }

    manager.Particles = particles
}

type Particle struct {
    Model *tetra3d.Model
    Life int
    Movement tetra3d.Vector3
}

func playSong(yield coroutine.YieldFunc, engine *Engine, songPath string, settings SongSettings) error {
    song, err := MakeSong(engine.AudioContext, songPath, settings.Difficulty)
    if err != nil {
        return err
    }

    defer song.Close()

    scene := tetra3d.NewScene("Scene")
    scene.World.LightingOn = false

    makeMesh := func(color tetra3d.Color) *tetra3d.Mesh {
        mesh := NewCylinderMesh(15, 4, 3)
        tetra3d.NewVertexSelection().SelectMeshPartByName(mesh, "CylinderTop").SetColor(1, color)
        mesh.SetActiveColorChannel(1)
        return mesh
    }

    fretColor := func(fret int) tetra3d.Color {
        switch fret {
            case 0: return tetra3d.NewColor(1, 0, 0, 1)
            case 1: return tetra3d.NewColor(0, 1, 0, 1)
            case 2: return tetra3d.NewColor(1, 1, 0, 1)
            case 3: return tetra3d.NewColor(0, 0, 1, 1)
            case 4: return tetra3d.NewColor(1, 0.5, 0, 1)
            default: return tetra3d.NewColor(1, 1, 1, 1)
        }
    }

    redMesh := makeMesh(fretColor(0))
    greenMesh := makeMesh(fretColor(1))
    yellowMesh := makeMesh(fretColor(2))
    blueMesh := makeMesh(fretColor(3))
    orangeMesh := makeMesh(fretColor(4))

    neckMesh := make3dRectangle(70, 5, 350, tetra3d.NewColor(1, 1, 1, 1))
    neckModel := tetra3d.NewModel("Neck", neckMesh)
    neckModel.Color = tetra3d.NewColor(1, 1, 1, 1)
    neckModel.Move(0, -5, 40)

    neckFile, err := os.Open("guitar2.jpg")
    if err != nil {
        log.Printf("Unable to open neck texture file: %v", err)
    } else {
        neckTextureImg, err := loadJpeg(bufio.NewReader(neckFile))
        if err != nil {
            log.Printf("Unable to load neck texture image: %v", err)
        } else {
            neckMesh.MeshPartByMaterialName("Top").Material.Texture = neckTextureImg
        }
    }
    /*
    grey := ebiten.NewImage(1, 1)
    grey.Fill(color.NRGBA{R: 50, G: 50, B: 50, A: 255})
    neckMesh.MeshPartByMaterialName("Top").Material.Texture = grey
    */

    // flameManager := NewFlameManager()
    particleManager := NewParticleManager(scene)

    stripMesh := make3dRectangle(70, 1, 1, tetra3d.NewColor(1, 1, 1, 1))
    stripModel := tetra3d.NewModel("Strip", stripMesh)
    stripModel.Color = tetra3d.NewColor(1, 1, 1, 1)
    stripModel.Move(0, 5/2 + 0.1, -20)
    neckModel.AddChildren(stripModel)
    scene.Root.AddChildren(neckModel)
    // scene.Root.AddChildren(stripModel)

    /*
    playMesh := make3dRectangle(8, 1, 1, tetra3d.NewColor(1, 0, 0, 1))
    playModelLow := tetra3d.NewModel("PlayButton", playMesh)
    playModelLow.Color = tetra3d.NewColor(1, 1, 1, 1)
    playModelLow.Move(0, 0, -float32(NoteThresholdLow.Microseconds())/20000)
    scene.Root.AddChildren(playModelLow)

    log.Printf("Low: %v", playModelLow.WorldPosition().Z)

    play2Mesh := make3dRectangle(8, 1, 1, tetra3d.NewColor(1, 1, 0, 1))
    playModelHigh := tetra3d.NewModel("PlayButton", play2Mesh)
    playModelHigh.Color = tetra3d.NewColor(1, 1, 1, 1)
    playModelHigh.Move(0, 0, -float32(NoteThresholdHigh.Microseconds())/20000)
    tetra3d.NewVertexSelection().SelectMeshPartByName(play2Mesh, "Top").SetColor(1, tetra3d.NewColor(1, 1, 1, 1))
    scene.Root.AddChildren(playModelHigh)

    log.Printf("High: %v", playModelHigh.WorldPosition().Z)

    zeroMesh := make3dRectangle(3, 1, 1, tetra3d.NewColor(1, 1, 1, 1))
    zeroModel := tetra3d.NewModel("ZeroMarker", zeroMesh)
    zeroModel.Color = tetra3d.NewColor(1, 1, 1, 1)
    zeroModel.Move(0, 0, 0)
    scene.Root.AddChildren(zeroModel)
    */

    /*
    cubeX := tetra3d.NewCubeMesh()
    cubeModel := tetra3d.NewModel("Cube", cubeX)
    cubeModel.Color = tetra3d.NewColor(0, 1, 0, 1)
    cubeModel.Move(0, 0, 0)
    scene.Root.AddChildren(cubeModel)
    */

    makeButton := func(fret int, mesh *tetra3d.Mesh) *tetra3d.Model {
        button := tetra3d.NewModel("Button", mesh)
        button.Color = tetra3d.NewColor(1, 1, 1, 0.3)
        xPos := (fret - 2) * 10
        button.Move(float32(xPos), 0, 0)
        return button
    }

    redButton := makeButton(0, redMesh)
    greenButton := makeButton(1, greenMesh)
    yellowButton := makeButton(2, yellowMesh)
    blueButton := makeButton(3, blueMesh)
    orangeButton := makeButton(4, orangeMesh)

    /*
    redPressMesh := makeMesh(tetra3d.NewColor(1, 0, 0, 1))
    redPress := tetra3d.NewModel("Debug", redPressMesh)
    redPress.Color = tetra3d.NewColor(1, 1, 1, 0.5)
    redPress.Move(-20, -1, 0)
    */

    for _, button := range []*tetra3d.Model{redButton, greenButton, yellowButton, blueButton, orangeButton} {
        scene.Root.AddChildren(button)
    }

    camera := tetra3d.NewCamera(ScreenWidth, ScreenHeight)
    camera.SetFar(310)
    // camera := tetra3d.NewCamera(300, 300)
    camera.SetFieldOfView(90)
    // camera.SetLocalPosition(0, 10, 500)
    camera.Move(0, 30, 40)
    camera.Rotate(3.5, 0, 0, -0.3)
    // camera.Node.Move(tetra3d.NewVector3(0, 0, -10))
    // camera.SetLocalRotation(tetra3d.NewMatrix4Rotate(0, 0, 0, 2))

    scene.Root.AddChildren(camera)

    // pressedColor := tetra3d.NewColor(1, 1, 1, 1)
    // unpressedColor := tetra3d.NewColor(1, 1, 1, 0.5)

    meshes := []*tetra3d.Mesh{redMesh, greenMesh, yellowMesh, blueMesh, orangeMesh}

    type NoteModel struct {
        Model *tetra3d.Model
        Note *Note
        SustainModel *tetra3d.Model
    }

    var notes []NoteModel
    for fretI := range song.Frets {
        fret := &song.Frets[fretI]
        for i := range fret.Notes {
            note := &fret.Notes[i]
            model := tetra3d.NewModel("NoteRed", meshes[fretI])
            model.Color = tetra3d.NewColor(1, 1, 1, 1)

            xPos := (fretI - 2) * 10

            model.Move(float32(xPos), 0, float32(-note.Start.Milliseconds() / 50))
            scene.Root.AddChildren(model)

            noteModel := NoteModel{Model: model, Note: note}

            if note.HasSustain() {
                sustainMesh := make3dRectangle(4, 1, float32(note.End.Microseconds() - note.Start.Microseconds()) / 20000, fretColor(fretI))
                sustainModel := tetra3d.NewModel("Sustain", sustainMesh)
                sustainModel.Color = tetra3d.NewColor(1, 1, 1, 1)
                noteModel.SustainModel = sustainModel
                // sustainModel.Move(float32(xPos), 0, float32(-note.Start.Microseconds()/20000) - (float32(note.End.Microseconds() - note.Start.Microseconds()) / 40000))
                // scene.Root.AddChildren(sustainModel)
                model.AddChildren(sustainModel)
            }

            notes = append(notes, noteModel)
        }
    }

    // rotation := float32(0)

    engine.PushDrawer(func(screen *ebiten.Image) {
        engine.DrawSong3d(screen, song, scene, camera)
        // drawSong(screen, song, engine.Font)
    })
    defer engine.PopDrawer()

    // model.SetLocalRotation(model.LocalRotation().Rotated(1, 0, 1, 1.05))

    var counter uint64

    song.Update(engine.Input, particleManager)
    for !song.Finished() {
        counter += 1

        /*
        if (counter/5) % 30 < 5 {
            flameManager.MakeFlame(int((counter/5) % 30))
        }
        */
        /*
        if counter % 90 == 0 {
            particleManager.MakeFlame(0)
        }
        */

        keys := inpututil.AppendJustPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyEscape, ebiten.KeyCapsLock:
                    yield()
                    return nil
            }
        }

        /*
        rotation += 0.01
        camera.SetLocalRotation(tetra3d.NewMatrix4Rotate(1, 0, 1, rotation))
        */

        // model.Move(0, 0, 0.1)

        delta := time.Since(song.StartTime)

        song.Update(engine.Input, particleManager)

        var notesOut []NoteModel

        // log.Printf("Notes: %v", len(notes))
        for _, noteModel := range notes {
            if (noteModel.Note.State == NoteStateHit && !noteModel.Note.HasSustain()) || noteModel.Note.End - delta < -time.Second * 1 {
                scene.Root.RemoveChildren(noteModel.Model)
            } else {

                if noteModel.Note.State == NoteStateHit && noteModel.Note.HasSustain() {
                    noteModel.Model.Color.A = 0

                    z := min(1, max(0, float32(noteModel.Note.End - delta) / float32(noteModel.Note.End - noteModel.Note.Start)))

                    noteModel.SustainModel.SetLocalScale(1, 1, z)
                    position := noteModel.Model.WorldPosition()
                    noteModel.Model.SetWorldPosition(position.X, position.Y, 0)
                } else {
                    elapsed := noteModel.Note.Start - delta

                    position := noteModel.Model.WorldPosition()
                    x := position.X
                    y := position.Y

                    alpha := float32(1.0)
                    if elapsed > 0 {
                        alpha = min(1, float32(time.Second * 2) / float32(elapsed))
                    }

                    noteModel.Model.Color.A = alpha

                    noteModel.Model.SetWorldPosition(x, y, float32(float64(-(elapsed.Microseconds())) / 20000))
                }

                notesOut = append(notesOut, noteModel)
            }

            // noteModel.Move(0, 0, 0.2)
        }

        notes = notesOut

        // model.SetLocalRotation(model.LocalRotation().Rotated(0.5, 0.2, 0.5, 0.02))

        particleManager.Update()

        for i := range song.Frets {
            fret := &song.Frets[i]
            var button *tetra3d.Model
            switch i {
                case 0: button = redButton
                case 1: button = greenButton
                case 2: button = yellowButton
                case 3: button = blueButton
                case 4: button = orangeButton
            }
            if !fret.Press.IsZero() {
                button.Color.A = min(1, button.Color.A + 0.06)
                button.SetLocalScale(1, 1, 1)
            } else {
                button.Color.A = max(0.3, button.Color.A - 0.01)
                button.SetLocalScale(1, 0.3, 1)
            }
        }

        if yield() != nil {
            break
        }
    }

    log.Printf("Song finished! Notes hit: %d, Notes missed: %d", song.NotesHit, song.NotesMissed)

    return nil
}

// true if the directory contains song.ogg, guitar.ogg, and notes.mid
func isSongDirectory(path string) bool {
    hasSong := false
    hasGuitar := false
    hasNotes := false

    entries, err := os.ReadDir(path)
    if err != nil {
        return false
    }

    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }

        name := strings.ToLower(entry.Name())
        switch name {
            case "song.ogg", "song.mp3": hasSong = true
            case "guitar.ogg", "guitar.mp3": hasGuitar = true
            case "notes.mid": hasNotes = true
        }
    }

    return hasSong && hasGuitar && hasNotes
}

func scanSongs(where string, depth int) []string {

    if depth > 10 {
        return nil
    }

    var paths []string

    useFs := os.DirFS(where)

    fs.WalkDir(useFs, ".", func(path string, entry fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        fullPath := filepath.Join(where, path)

        if entry.IsDir() {
            if isSongDirectory(fullPath) {
                paths = append(paths, fullPath)
            }
            return nil
        } else {
            // might be a symlink to a directory
            info, err := fs.Stat(useFs, fullPath)
            if err == nil {
                if info.IsDir() {
                    if isSongDirectory(fullPath) {
                        paths = append(paths, fullPath)
                    }

                    paths = append(paths, scanSongs(fullPath, depth + 1)...)
                }
            }

            return nil
        }
    })

    return paths

    /*
    return []string{
        "Queen - Killer Queen",
        "CloneHeroSongs/Yes - Roundabout",
    }
    */
}

func loadPng(file io.Reader) (*ebiten.Image, error) {
    img, err := png.Decode(file)
    if err != nil {
        return nil, err
    }
    return ebiten.NewImageFromImage(img), nil
}

func loadJpeg(file io.Reader) (*ebiten.Image, error) {
    img, err := jpeg.Decode(file)
    if err != nil {
        return nil, err
    }
    return ebiten.NewImageFromImage(img), nil
}

func loadAlbumImage(songFS fs.FS) *ebiten.Image {

    possible := map[string]bool{
        "album.png": true,
        "album.jpg": true,
        "album.jpeg": true,
    }

    entries, err := fs.ReadDir(songFS, ".")
    if err != nil {
        return ebiten.NewImage(1, 1)
    }

    for _, entry := range entries {
        name := strings.ToLower(entry.Name())
        if entry.IsDir() {
            continue
        }

        if possible[name] {
            file1, err := songFS.Open(entry.Name())
            if err == nil {
                defer file1.Close()

                newImage, err := loadPng(file1)
                if err == nil {
                    return newImage
                }

            }

            file2, err := songFS.Open(entry.Name())
            if err == nil {
                defer file2.Close()
                newImage, err := loadJpeg(file2)
                if err == nil {
                    return newImage
                }
            }
        }
    }

    return ebiten.NewImage(1, 1)
}

func (engine *Engine) Draw(screen *ebiten.Image) {
    if len(engine.Drawers) > 0 {
        drawer := engine.Drawers[len(engine.Drawers)-1]
        drawer(screen)
    }
}

func brightenColor(c color.NRGBA, amount float64) color.Color {
    h, s, v := colorconv.ColorToHSV(c)

    v = v + (1.0 - v) * amount
    s = s - s * amount

    if v > 1.0 {
        v = 1.0
    }

    if s < 0.0 {
        s = 0.0
    }

    out, err := colorconv.HSVToColor(h, s, v)
    if err == nil {
        return out
    }

    return c
}

func darkenColor(c color.NRGBA, amount float64) color.Color {
    h, s, v := colorconv.ColorToHSV(c)
    v = v * (1.0 - amount)

    out, err := colorconv.HSVToColor(h, s, v)
    if err == nil {
        return out
    }

    return c
}

func (engine *Engine) DrawSong3d(screen *ebiten.Image, song *Song, scene *tetra3d.Scene, camera *tetra3d.Camera) {

    camera.Clear()
    camera.RenderScene(scene)
    screen.DrawImage(camera.ColorTexture(), nil)

    /*
    for _, flame := range flameManager.Flames {
        useImage := flame.Image()
        x := 695 + (flame.Fret - 2) * 120 - useImage.Bounds().Dx()/2
        y := ScreenHeight - 285 - useImage.Bounds().Dy()
        var options ebiten.DrawImageOptions
        options.GeoM.Translate(float64(x), float64(y))
        screen.DrawImage(useImage, &options)
    }
    */

    // camera.DrawDebugText(screen, "just a test", 0, 10, 2, tetra3d.NewColor(1, 1, 1, 1))

    delta := time.Since(song.StartTime)

    face := &text.GoTextFace{
        Source: engine.Font,
        Size: 24,
    }

    var textOptions text.DrawOptions
    textOptions.GeoM.Translate(850, 100)
    text.Draw(screen, fmt.Sprintf("Time: %v / %v", delta.Truncate(time.Second), song.SongLength.Truncate(time.Second)), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    text.Draw(screen, fmt.Sprintf("Notes Hit: %d", song.NotesHit), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    text.Draw(screen, fmt.Sprintf("Notes Missed: %d", song.NotesMissed), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    percent := 0
    if song.TotalNotes() > 0 {
        percent = song.NotesHit * 100 / song.TotalNotes()
    }
    text.Draw(screen, fmt.Sprintf("Notes: %d%%", percent), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    text.Draw(screen, fmt.Sprintf("Score: %d", song.Score), face, &textOptions)

}

// vertical layout
// func (engine *Engine) Draw3(screen *ebiten.Image) {
func drawSong(screen *ebiten.Image, song *Song, font *text.GoTextFaceSource) {
    // ebitenutil.DebugPrintAt(screen, fmt.Sprintf("FPS: %.2f", ebiten.ActualFPS()), 10, 10)

    delta := time.Since(song.StartTime)

    face := &text.GoTextFace{
        Source: font,
        Size: 24,
    }

    var textOptions text.DrawOptions
    textOptions.GeoM.Translate(850, 100)
    text.Draw(screen, fmt.Sprintf("Time: %v / %v", delta.Truncate(time.Second), song.SongLength.Truncate(time.Second)), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    text.Draw(screen, fmt.Sprintf("Notes Hit: %d", song.NotesHit), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    text.Draw(screen, fmt.Sprintf("Notes Missed: %d", song.NotesMissed), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    percent := 0
    if song.TotalNotes() > 0 {
        percent = song.NotesHit * 100 / song.TotalNotes()
    }
    text.Draw(screen, fmt.Sprintf("Notes: %d%%", percent), face, &textOptions)
    textOptions.GeoM.Translate(0, 30)
    text.Draw(screen, fmt.Sprintf("Score: %d", song.Score), face, &textOptions)

    playLine := ScreenHeight - 130

    white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
    grey := color.NRGBA{R: 100, G: 100, B: 100, A: 255}

    highwayXStart := 250

    xStart := 50 + highwayXStart
    xWidth := 500

    vector.FillRect(screen, float32(xStart), 0, float32(xWidth), float32(ScreenHeight), color.NRGBA{R: 32, G: 32, B: 32, A: 255}, true)

    vector.StrokeLine(screen, float32(xStart), float32(playLine), float32(xStart + xWidth), float32(playLine), 5, white, true)

    const noteSpeed = 8

    thresholdLow := int64(NoteThresholdLow / time.Millisecond) / noteSpeed
    thresholdHigh := int64(NoteThresholdHigh / time.Millisecond) / noteSpeed

    vector.FillRect(screen, float32(xStart), float32(playLine - int(thresholdHigh)), float32(xWidth), float32(int(thresholdHigh - thresholdLow)), color.NRGBA{R: 100, G: 100, B: 100, A: 100}, true)

    const noteSize = 25

    for i := range song.Frets {
        fret := &song.Frets[i]

        xFret := 100 + highwayXStart + i * 100

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

                var useColor color.Color = fretColor

                if note.State == NoteStateMissed || (note.State == NoteStateHit && !note.Sustain) {
                    useColor = darkenColor(fretColor, 0.5)
                }

                vector.FillCircle(screen, float32(xFret), float32(y), noteSize, useColor, true)

                if note.State == NoteStateHit {
                    vector.StrokeCircle(screen, float32(xFret), float32(y), noteSize + 1, 2, white, false)
                } else if note.State == NoteStateMissed {
                    vector.StrokeCircle(screen, float32(xFret), float32(y), noteSize + 1, 2, grey, false)
                }

                if -(end - start) > 200 / noteSpeed {
                    thickness := 10

                    x1 := xFret - thickness / 2
                    y1 := end

                    vector.FillRect(screen, float32(x1), float32(y1), float32(thickness), float32(-(end - start)), useColor, true)

                    if note.State == NoteStateHit && note.Sustain {
                        vector.StrokeRect(screen, float32(x1), float32(y1), float32(thickness), float32(-(end - start)), 2, white, false)
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

    if delta < time.Second * 2 && song.SongInfo.Name != "" {
        face = &text.GoTextFace{
            Source: font,
            Size: 30,
        }

        if delta < time.Millisecond * 300 {
            textOptions.ColorScale.ScaleAlpha(float32(delta) / float32(time.Millisecond * 300))
        } else if delta > time.Millisecond * 1700 {
            diff := delta - time.Millisecond * 1700
            alpha := float32(time.Millisecond * 300 - diff) / float32(time.Millisecond * 300)
            if alpha < 0 {
                alpha = 0
            }
            textOptions.ColorScale.ScaleAlpha(alpha)
        }

        textOptions.GeoM.Reset()
        textOptions.GeoM.Translate(10, 60)
        text.Draw(screen, fmt.Sprintf("Song: %s", song.SongInfo.Name), face, &textOptions)
        textOptions.GeoM.Translate(0, 40)
        text.Draw(screen, fmt.Sprintf("Artist: %s", song.SongInfo.Artist), face, &textOptions)
    }
}

// horizontal layout
/*
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
*/

func (engine *Engine) Layout(outsideWidth, outsideHeight int) (int, int) {
    return ScreenWidth, ScreenHeight
}

func main() {
    log.SetFlags(log.Ldate | log.Lshortfile | log.Lmicroseconds)

    var path string

    if len(os.Args) > 1 {
        path = os.Args[1]
    }

    log.Printf("Initializing")

    ebiten.SetTPS(120)
    ebiten.SetWindowSize(ScreenWidth, ScreenHeight)
    ebiten.SetWindowTitle("Rhythm Game")
    ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

    audioContext := audio.NewContext(44100)

    engine, err := MakeEngine(audioContext, path)

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
