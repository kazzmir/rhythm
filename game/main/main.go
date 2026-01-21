package main

import (
    "log"
    "time"
    "image"
    "math"
    "math/rand/v2"
    "strconv"
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
    // "github.com/hajimehoshi/ebiten/v2/ebitenutil"
    "github.com/hajimehoshi/ebiten/v2/text/v2"

    "github.com/kazzmir/opus-go/ogg"
    "github.com/kazzmir/opus-go/opus"

    "github.com/solarlune/tetra3d"
)

const ScreenWidth = 1400
const ScreenHeight = 1000

type ConfigurationManager struct {
}

func (config *ConfigurationManager) LoadInputProfile() *InputProfile {
    file, err := os.Open("config.json")
    if err == nil {
        defer file.Close()
        buffer := bufio.NewReader(file)

        profile, err := LoadInputProfile(buffer)
        if err == nil {
            return profile
        } else {
            log.Printf("Failed to load input profile from config.json: %v", err)
        }
    }

    return NewInputProfile()
}

func (config *ConfigurationManager) SaveConfiguration(doSave func (io.Writer) error) error {
    file, err := os.Create("config.json")
    if err != nil {
        return err
    }

    defer file.Close()

    buffer := bufio.NewWriter(file)
    defer buffer.Flush()

    return doSave(buffer)
}

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

type Lyric struct {
    Time time.Duration
    Text string
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

func (action InputAction) String() string {
    switch action {
        case InputActionGreen: return "Green"
        case InputActionRed: return "Red"
        case InputActionYellow: return "Yellow"
        case InputActionBlue: return "Blue"
        case InputActionOrange: return "Orange"
        case InputActionStrumUp: return "Strum Up"
        case InputActionStrumDown: return "Strum Down"
        default: return "Unknown"
    }
}

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

type LyricBatch []Lyric
func (batch LyricBatch) StartTime() time.Duration {
    return batch[0].Time
}

func (batch LyricBatch) EndTime() time.Duration {
    return batch[len(batch)-1].Time
}

type Part struct {
    Name string
    Player *audio.Player
}

type Song struct {
    Frets []Fret
    StartTime time.Time
    CleanupFuncs []func()

    LyricBatches []LyricBatch

    LyricBatch int

    Parts []Part
    /*
    Song *audio.Player
    Guitar *audio.Player
    Vocals *audio.Player
    */

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
    for _, part := range song.Parts {
        part.Player.Pause()
        part.Player.Close()
    }

    /*
    song.Song.Pause()
    song.Song.Close()
    song.Guitar.Pause()
    song.Guitar.Close()

    if song.Vocals != nil {
        song.Vocals.Pause()
        song.Vocals.Close()
    }
    */

    for _, cleanup := range song.CleanupFuncs {
        cleanup()
    }
}

func (song *Song) Update(input *InputProfile, flameMaker FlameMaker) {
    song.Counter += 1

    song.DoSong.Do(func(){
        for _, part := range song.Parts {
            part.Player.Play()
        }

        /*
        song.Song.Play()
        song.Guitar.Play()
        if song.Vocals != nil {
            song.Vocals.Play()
        }
        */
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

        if input.IsJustPressed(fret.InputAction) {
            fret.Press = time.Now()
        } else if input.IsJustReleased(fret.InputAction) {
            fret.Press = time.Time{}
        }

        /*
        key := input.GetKeyboardButton(fret.InputAction)
        if inpututil.IsKeyJustPressed(key) {
            fret.Press = time.Now()
        } else if inpututil.IsKeyJustReleased(key) {
            fret.Press = time.Time{}
        }

        if input.HasGamepad() {
            button := input.GetGamepadButton(fret.InputAction)
            if inpututil.IsGamepadButtonJustPressed(input.CurrentGamepadProfile.GamepadID, button) {
                fret.Press = time.Now()
            } else if inpututil.IsGamepadButtonJustReleased(input.CurrentGamepadProfile.GamepadID, button) {
                fret.Press = time.Time{}
            }
        }
        */
    }

    playGuitar := false
    stopGuitar := false
    changeGuitar := false

    // when true, we don't need to strum
    allTapsMode := false

    forceMiss := false

    var notesHit []*Note

    strummed := input.IsJustPressed(InputActionStrumDown)

    /*
    strummed := inpututil.IsKeyJustPressed(input.GetKeyboardButton(InputActionStrumDown))
    if input.HasGamepad() {
        button := input.GetGamepadButtons(InputActionStrumDown)
        strummed = strummed || inpututil.IsGamepadButtonJustPressed(input.CurrentGamepadProfile.GamepadID, button)
    }
    */

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
            pressed = input.IsJustPressed(fret.InputAction)
            /*
            key := input.GetKeyboardButton(fret.InputAction)
            pressed = inpututil.IsKeyJustPressed(key)
            if input.HasGamepad() {
                button := input.GetGamepadButton(fret.InputAction)
                if inpututil.IsGamepadButtonJustPressed(input.CurrentGamepadProfile.GamepadID, button) {
                    pressed = true
                }
            }
            */
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
        var guitarPart *audio.Player
        for _, part := range song.Parts {
            if part.Name == "guitar" {
                guitarPart = part.Player
            }
        }

        if guitarPart != nil {

            if playGuitar && !stopGuitar {

                guitarPart.SetVolume(1.0)

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
                guitarPart.SetVolume(0.3)
                // song.Guitar.Pause()
            }
        }
    }

    for song.LyricBatch < len(song.LyricBatches) && delta >= song.LyricBatches[song.LyricBatch].EndTime() + time.Millisecond * 300 {
        song.LyricBatch += 1
    }
}

// returns the index of the guitar track, or -1 if not found
func findTrackByName(smf *smflib.SMF, name string) int {
    for i, track := range smf.Tracks {
        if len(track) > 0 {
            event := track[0]
            var trackName string
            if event.Message.GetMetaTrackName(&trackName) {
                if strings.Contains(strings.ToLower(trackName), name) {
                    return i
                }
            }
        }
    }

    return -1
}

// returns the index of the guitar track, or -1 if not found
func findGuitarTrack(smf *smflib.SMF) int {
    // the track name is usually "part guitar", but we just care about the guitar part
    return findTrackByName(smf, "guitar")
}

func findVocalsTrack(smf *smflib.SMF) int {
    return findTrackByName(smf, "vocals")
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

func loadAudio(audioContext *audio.Context, basefs fs.FS, baseName string) (*audio.Player, time.Duration, func(), error) {
    type Loader struct {
        Name string
        LoadFunc func(*audio.Context, fs.File, string) (*audio.Player, time.Duration, func(), error)
    }

    loaders := []Loader{
        Loader{
            Name: fmt.Sprintf("%v.ogg", baseName),
            LoadFunc: loadOgg,
        },
        Loader{
            Name: fmt.Sprintf("%v.mp3", baseName),
            LoadFunc: loadMp3,
        },
        Loader{
            Name: fmt.Sprintf("%v.opus", baseName),
            LoadFunc: loadOpus,
        },
    }

    for _, loader := range loaders {
        songFile, err := findFile(basefs, loader.Name)
        if err == nil {
            defer songFile.Close()

            var cleanup func()

            songPlayer, songLength, cleanup, err := loader.LoadFunc(audioContext, songFile, loader.Name)
            if err != nil {
                return nil, 0, nil, fmt.Errorf("Unable to create audio player for file '%v': %v", loader.Name, err)
            }

            return songPlayer, songLength, cleanup, nil
        }
    }

    return nil, 0, nil, fmt.Errorf("Unable to find song.ogg or song.mp3 in song directory")
}

func loadSong(audioContext *audio.Context, basefs fs.FS) (*audio.Player, time.Duration, func(), error) {
    return loadAudio(audioContext, basefs, "song")
}

func loadGuitarSong(audioContext *audio.Context, basefs fs.FS) (*audio.Player, func(), error) {
    player, _, cleanup, err := loadAudio(audioContext, basefs, "guitar")
    return player, cleanup, err
}

func loadVocalSong(audioContext *audio.Context, basefs fs.FS) (*audio.Player, func(), error) {
    player, _, cleanup, err := loadAudio(audioContext, basefs, "vocals")
    return player, cleanup, err
}

func isAudioFile(name string) bool {
    ext := strings.ToLower(filepath.Ext(name))
    switch ext {
        case ".mp3", ".ogg", ".opus":
            return true
        default:
            return false
    }
}

func loadAudio2(audioContext *audio.Context, basefs fs.FS, name string, ext string) (*audio.Player, time.Duration, func(), error) {
    file, err := basefs.Open(name)
    if err != nil {
        return nil, 0, nil, fmt.Errorf("Unable to open audio file '%v': %v", name, err)
    }
    defer file.Close()

    switch ext {
        case ".mp3": return loadMp3(audioContext, file, name)
        case ".ogg": return loadOgg(audioContext, file, name)
        case ".opus": return loadOpus(audioContext, file, name)
    }

    return nil, 0, nil, fmt.Errorf("Unsupported audio file extension '%v' for file '%v'", ext, name)
}

// find all audio files in the basefs
func loadSongParts(audioContext *audio.Context, basefs fs.FS) ([]Part, time.Duration, []func(), error) {
    entries, err := fs.ReadDir(basefs, ".")
    if err != nil {
        return nil, 0, nil, fmt.Errorf("Unable to read song directory: %v", err)
    }

    var longest time.Duration
    var parts []Part
    var cleanupFuncs []func()

    for _, entry := range entries {
        name := entry.Name()

        if isAudioFile(name) {
            player, duration, cleanup, err := loadAudio2(audioContext, basefs, name, strings.ToLower(filepath.Ext(name)))
            if err == nil {
                parts = append(parts, Part{
                    Name: strings.TrimSuffix(name, filepath.Ext(name)),
                    Player: player,
                })
                cleanupFuncs = append(cleanupFuncs, cleanup)
                if duration > longest {
                    longest = duration
                }
            }
        }
    }

    return parts, longest, cleanupFuncs, nil
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

    // var songLength time.Duration
    // var songPlayer *audio.Player
    var err error

    song.Parts, song.SongLength, song.CleanupFuncs, err = loadSongParts(audioContext, basefs)
    if err != nil {
        return nil, fmt.Errorf("Unable to load song parts: %v", err)
    }

    /*
    songPlayer, songLength, cleanup, err := loadSong(audioContext, basefs)
    if err != nil {
        return nil, fmt.Errorf("Unable to load song audio: %v", err)
    }
    song.CleanupFuncs = append(song.CleanupFuncs, cleanup)

    guitarPlayer, cleanup, err := loadGuitarSong(audioContext, basefs)
    if err != nil {
        return nil, fmt.Errorf("Unable to load guitar audio: %v", err)
    }
    song.CleanupFuncs = append(song.CleanupFuncs, cleanup)

    vocalsPlayer, cleanup, err := loadVocalSong(audioContext, basefs)
    if err != nil {
        vocalsPlayer = nil
    }
    */

    // song.SongLength = songLength
    /*
    song.Song = songPlayer
    song.Guitar = guitarPlayer
    song.Vocals = vocalsPlayer
    */

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

    err = song.ReadNotes(notesData, difficulty, song.SongLength)
    if err != nil {
        return nil, err
    }

    err = song.ReadLyrics(notesData)

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
                case "song_length":
                    v, err := strconv.ParseInt(value, 10, 64)
                    if err == nil {
                        out.SongLength = time.Millisecond * time.Duration(v)
                    }
            }
        }
    }

    return out
}

// notesData is assumed to be the contents of a MIDI file
func (song *Song) ReadLyrics(notesData []byte) error {
    smf, err := smflib.ReadFrom(bytes.NewReader(notesData))
    if err != nil {
        return fmt.Errorf("Unable to read MIDI file '%v': %v", "notes.mid", err)
    }

    vocalsTrack := findVocalsTrack(smf)
    if vocalsTrack == -1 {
        return fmt.Errorf("no vocals track")
    }

    reader := smflib.ReadTracksFrom(bytes.NewReader(notesData), vocalsTrack)
    if reader.Error() != nil {
        return reader.Error()
    }

    var lyrics []Lyric

    reader.Do(func (event smflib.TrackEvent) {
        // log.Printf("Vocals track event: %v", event)
        var text string
        if event.Message.GetMetaLyric(&text) {
            // log.Printf("Lyric at %v: %v", time.Microsecond * time.Duration(event.AbsMicroSeconds), text)

            lyrics = append(lyrics, Lyric{
                Time: time.Microsecond * time.Duration(event.AbsMicroSeconds),
                Text: text,
            })
        }
    })

    var currentBatch []Lyric
    for _, lyric := range lyrics {
        if len(currentBatch) == 0 {
            currentBatch = append(currentBatch, lyric)
        } else {
            if lyric.Time - currentBatch[0].Time < time.Millisecond * 2000 {
                currentBatch = append(currentBatch, lyric)
            } else {
                song.LyricBatches = append(song.LyricBatches, currentBatch)
                currentBatch = []Lyric{lyric}
            }
        }
    }

    return nil
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
            low = 84
            high = 88
        case "expert":
            low = 96
            high = 100
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
    Ticks int

    Drawers []func(screen *ebiten.Image)

    Coroutine *coroutine.Coroutine
    Configuration *ConfigurationManager

    // GamepadIds map[ebiten.GamepadID]struct{}

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

func (engine *Engine) LastDrawer() func(screen *ebiten.Image) {
    if len(engine.Drawers) > 0 {
        return engine.Drawers[len(engine.Drawers)-1]
    }

    // dummy
    return func(screen *ebiten.Image) {}
}

func loadMp3(audioContext *audio.Context, file fs.File, name string) (*audio.Player, time.Duration, func(), error) {
    allData, err := io.ReadAll(bufio.NewReader(file))
    if err != nil {
        return nil, 0, nil, err
    }

    // this decodes on the fly, so we need all the data in memory when we pass a reader
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

type opusWrapper struct {
    reader *ogg.OpusReader
    decoder *opus.Decoder
    buffer []int16
    preskipRemaining int
    position int
}

func (wrapper *opusWrapper) Read(p []byte) (int, error) {
    // we have len(p) bytes to fill up, which is len(p)/2 int16 samples
    // we are going to produce stereo audio, so the number of samples read per channel will be len(p)/4

    if wrapper.position >= len(wrapper.buffer) {
        packet, err := wrapper.reader.ReadAudioPacket()
        if err != nil {
            // fill with silence
            for i := range p {
                p[i] = 0
            }

            return len(p), err
        }

        wrapper.buffer = wrapper.buffer[:cap(wrapper.buffer)]
        wrapper.position = 0

        decoded, n, err := wrapper.decoder.DecodePacket(packet, wrapper.buffer)
        if err != nil {
            return 0, err
        }

        wrapper.buffer = decoded
        if wrapper.preskipRemaining > 0 {
            skip := wrapper.preskipRemaining
            if skip > n {
                skip = n
            }
            wrapper.preskipRemaining -= skip
            wrapper.position += skip * int(wrapper.reader.Head.Channels)
        }
    }

    switch wrapper.reader.Head.Channels {
        case 1:
            // we have to produce stereo audio, so each input sample becomes two output samples
            atMost := min(len(p) / 4, len(wrapper.buffer) - wrapper.position)
            // log.Printf("Rendering opus: p=%d buffer=%d atMost=%d position=%d", len(p), len(wrapper.buffer), atMost, wrapper.position)
            count := 0
            for count < atMost {
                low := byte(wrapper.buffer[wrapper.position + count] & 0xFF)
                high := byte((wrapper.buffer[wrapper.position + count] >> 8) & 0xFF)

                p[count*4] = low
                p[count*4+1] = high
                p[count*4+2] = low
                p[count*4+3] = high
                count += 1
            }
            wrapper.position += count

            return count * 4, nil

        case 2:
            atMost := min(len(p) / 2, len(wrapper.buffer) - wrapper.position)

            // log.Printf("Rendering opus: p=%d buffer=%d atMost=%d position=%d", len(p), len(wrapper.buffer), atMost, wrapper.position)

            count := 0
            for count < atMost {
                p[count*2] = byte(wrapper.buffer[wrapper.position + count] & 0xFF)
                p[count*2+1] = byte((wrapper.buffer[wrapper.position + count] >> 8) & 0xFF)
                count += 1
            }
            wrapper.position += count

            return count * 2, nil
    }

    return 0, fmt.Errorf("unsupported number of channels: %d", wrapper.reader.Head.Channels)
}

func loadOpus(audioContext *audio.Context, file fs.File, name string) (*audio.Player, time.Duration, func(), error) {
    allData, err := io.ReadAll(bufio.NewReader(file))
    if err != nil {
        return nil, 0, nil, err
    }

    reader, err := ogg.NewOpusReader(bytes.NewReader(allData))
    if err != nil {
        return nil, 0, nil, err
    }

    reader2, _ := ogg.NewOpusReader(bytes.NewReader(allData))
    totalSamples, err := reader2.TotalSamples()
    if err != nil {
        return nil, 0, nil, err
    }

    decoder, err := opus.NewDecoderFromHead(reader.Head)
    if err != nil {
        return nil, 0, nil, err
    }

    wrapper := &opusWrapper{
        reader: reader,
        decoder: decoder,
        preskipRemaining: int(reader.Head.PreSkip),
    }

    player, err := audioContext.NewPlayer(audio.ResampleReader(wrapper, totalSamples * int64(reader.Head.Channels), ogg.OpusSampleRateHz, audioContext.SampleRate()))

    if err != nil {
        return nil, 0, nil, err
    }

    return player, time.Duration(totalSamples) * time.Second / ogg.OpusSampleRateHz, func(){}, nil
}

func loadOgg(audioContext *audio.Context, file fs.File, name string) (*audio.Player, time.Duration, func(), error) {
    allData, err := io.ReadAll(bufio.NewReader(file))
    if err != nil {
        return nil, 0, nil, err
    }

    // this decodes on the fly, so we need all the data in memory when we pass a reader
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

func MakeEngine(audioContext *audio.Context, songDirectory string, ticks int) (*Engine, error) {
    font, err := LoadFont()
    if err != nil {
        return nil, fmt.Errorf("Failed to load font: %v", err)
    }

    var engine *Engine

    engine = &Engine{
        AudioContext: audioContext,
        Font: font,
        Ticks: ticks,
        // GamepadIds: make(map[ebiten.GamepadID]struct{}),
        Coroutine: coroutine.MakeCoroutine(func(yield coroutine.YieldFunc) error {
            if songDirectory != "" {
                err := playSong(yield, engine, songDirectory, DefaultSongSettings(), NewInputProfile())
                return err
            }

            return mainMenu(engine, yield)
        }),
        Configuration: &ConfigurationManager{},
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

    /*
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
    */

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

		pos := tetra3d.NewVector3(radius, height, 0)
		pos = pos.Rotate(0, 1, 0, float32(i)/float32(sideCount)*math.Pi*2)
        topVerts = append(topVerts, len(verts))
		verts = append(verts, tetra3d.NewVertex(pos.X, pos.Y, pos.Z, 0, 0))

	}

    var bottomVerts []int
	for i := 0; i < sideCount; i++ {

		pos := tetra3d.NewVector3(radius, 0, 0)
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

// create a flat plane mesh that is made up of smaller rectangles so that UV texturing looks smooth
func makePlane(width, depth int, color tetra3d.Color) *tetra3d.Mesh {
    mesh := tetra3d.NewMesh("Plane")

    x0 := 0
    x1 := width
    z0 := 0
    z1 := depth

    adjustX := width / 2

    type Quad struct {
        // v0 upper left, v1 upper right, v2 lower left, v3 lower right
        v0, v1, v2, v3 int
    }

    var quads []Quad

    quadSize := 20

    points := make(map[image.Point]int)
    verts := []tetra3d.VertexInfo{}
    // index into verts
    // indexes := make(map[*tetra3d.VertexInfo]int)

    zf := func(z float32) float32 {
        // return z - 30
        return -z
        // return float32(depth)-z - 30
    }

    maxX := 0
    maxZ := 0

    x := x0
    z := z0
    for x = x0; x <= x1; x += quadSize {
        for z = z0; z <= z1; z += quadSize {
            v0 := tetra3d.NewVertex(float32(x - adjustX), 0, zf(float32(z)), float32(x) / float32(width), float32(z) / float32(depth))
            verts = append(verts, v0)
            index := len(verts) - 1
            points[image.Pt(x / quadSize, z / quadSize)] = index

            maxX = max(maxX, x / quadSize)
            maxZ = max(maxZ, z / quadSize)
        }

        // edge case for bottom
        if z != z1 + quadSize {
            v0 := tetra3d.NewVertex(float32(x - adjustX), 0, zf(float32(depth)), float32(x) / float32(width), 1)
            verts = append(verts, v0)
            index := len(verts) - 1
            points[image.Pt(x / quadSize, z / quadSize)] = index

            maxX = max(maxX, x / quadSize)
            maxZ = max(maxZ, z / quadSize)
        }
    }

    // handle edge case when right side is not multiple of quadSize
    if x != x1 + quadSize {
        for z = z0; z <= z1; z += quadSize {
            v0 := tetra3d.NewVertex(float32(width - adjustX), 0, zf(float32(z)), 1, float32(z) / float32(depth))
            verts = append(verts, v0)
            index := len(verts) - 1
            points[image.Pt(x / quadSize, z / quadSize)] = index
            maxX = max(maxX, x / quadSize)
            maxZ = max(maxZ, z / quadSize)
        }

        if z != z1 + quadSize {
            v0 := tetra3d.NewVertex(float32(width - adjustX), 0, zf(float32(depth)), 1, 1)
            verts = append(verts, v0)
            index := len(verts) - 1
            points[image.Pt(x / quadSize, z / quadSize)] = index
            maxX = max(maxX, x / quadSize)
            maxZ = max(maxZ, z / quadSize)
        }
    }

    for x := range maxX {
        for z := range maxZ {
            v0 := points[image.Pt(x, z)]
            v1 := points[image.Pt(x+1, z)]
            v2 := points[image.Pt(x+1, z+1)]
            v3 := points[image.Pt(x, z+1)]

            // quads = append(quads, Quad{v0: v0, v1: v1, v2: v2, v3: v3})
            quads = append(quads, Quad{v0: v1, v1: v0, v2: v3, v3: v2})
        }
    }

    // log.Printf("Made plane with %d verts and %d quads", len(verts), len(quads))
    // log.Printf("Quads: %+v", quads)
    // log.Printf("Vertices: %+v", verts)

    mesh.AddVertices(verts...)

    var indices []int
    for _, quad := range quads {
        indices = append(indices, tesselate(verts, []int{quad.v3, quad.v2, quad.v1, quad.v0})...)
    }
    mesh.AddMeshPart(tetra3d.NewMaterial("Top"), indices...)

    for _, part := range mesh.MeshParts {
        name := part.Material.Name()
        tetra3d.NewVertexSelection().SelectMeshPartByName(mesh, name).SetColor(1, color)
    }

    // tetra3d.NewVertexSelection().SelectIndices(mesh, 8).SetColor(1, tetra3d.NewColor(1, 0, 0, 1))

    mesh.SetActiveColorChannel(1)

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
    // topPlane := []int{2, 7, 3}
    bottomPlane := []int{0, 4, 5, 1}

    /*
    verts[topPlane[0]].U = 0
    verts[topPlane[0]].V = 0

    verts[topPlane[1]].U = 1
    verts[topPlane[1]].V = 1

    verts[topPlane[2]].U = 0
    verts[topPlane[2]].V = 1
    */

    /*
    verts[topPlane[3]].U = 0
    verts[topPlane[3]].V = 1
    */

    mesh.AddVertices(verts...)

    // tetra3d.NewVertexSelection().SelectIndices(mesh, topPlane[0]).SetColor(1, tetra3d.NewColor(1, 0, 0, 1))

    _ = frontPlane
    _ = rightPlane
    _ = leftPlane
    _ = backPlane
    _ = topPlane
    _ = bottomPlane

    mesh.AddMeshPart(tetra3d.NewMaterial("Front"), tesselate(verts, frontPlane)...)
    // mesh.AddMeshPart(tetra3d.NewMaterial("Right"), tesselate(verts, rightPlane)...)
    // mesh.AddMeshPart(tetra3d.NewMaterial("Left"), tesselate(verts, leftPlane)...)
    // log.Printf("Top plane: %v", tesselate(verts, topPlane))
    mesh.AddMeshPart(tetra3d.NewMaterial("Top"), tesselate(verts, topPlane)...)
    /*
    mesh.AddMeshPart(tetra3d.NewMaterial("Back"), tesselate(verts, backPlane)...)
    mesh.AddMeshPart(tetra3d.NewMaterial("Bottom"), tesselate(verts, bottomPlane)...)
    */

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
    color = tetra3d.NewColor(248.0/255.0, 134.0/255.0, 69.0/255.0, 1)

    for range newParticles {
        model := tetra3d.NewModel("Particle", manager.ParticleMesh)
        model.Color = color
        model.SetWorldPosition(float32((fret - 2) * 10), 0, 0)

        manager.Scene.Root.AddChildren(model)

        manager.Particles = append(manager.Particles, &Particle{
            Model: model,
            Life: 0,
            Movement: tetra3d.NewVector3((rand.Float32() - 0.5) / 3, 0.1 + rand.Float32() / 2, (rand.Float32() - 0.5) / 3),
        })
    }
}

func (manager *ParticleManager) Update() {
    particles := make([]*Particle, 0, len(manager.Particles))

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

func loadSkin() *ebiten.Image {

    base := "skins"
    files, err := data.SkinsFS.ReadDir(base)
    if err == nil {
        // choose one at random, but go through all of them until one loads
        for _, i := range rand.Perm(len(files)) {
            path := files[i]
            if path.IsDir() {
                continue
            }
            full := filepath.Join(base, path.Name())

            out := func() (*ebiten.Image, error) {
                file, err := data.SkinsFS.Open(full)
                if err == nil {
                    defer file.Close()
                    return loadJpeg(bufio.NewReader(file))
                }
                return nil, err
            }

            texture, err := out()
            if err == nil {
                return texture
            }
        }
    }

    // TODO: load user skins from "skins" directory on disk

    /*
    file, err := os.Open("skins/stars.jpg")
    if err != nil {
        log.Printf("Unable to open neck texture file: %v", err)
    } else {
        defer file.Close()
        texture, err := loadJpeg(bufio.NewReader(file))
        if err != nil {
            log.Printf("Unable to load neck texture image: %v", err)
        } else {
            return texture
        }
    }
    */

    // fallback
    grey := ebiten.NewImage(1, 1)
    grey.Fill(color.NRGBA{R: 50, G: 50, B: 50, A: 255})
    return grey
}

// return the rotation vector that looks from position to target
func lookAtVector(position tetra3d.Vector3, target tetra3d.Vector3) tetra3d.Vector3 {
    direction := target.Sub(position)
    magnitude := direction.Magnitude()

    f := direction.Divide(magnitude)
    yaw := math.Atan2(float64(f.X), float64(f.Z))
    pitch := math.Asin(float64(f.Y))

    return tetra3d.NewVector3(float32(pitch), float32(yaw), 0)
}

func playSong(yield coroutine.YieldFunc, engine *Engine, songPath string, settings SongSettings, input *InputProfile) error {
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

    timeToZ := func(t time.Duration) float32 {
        return float32(t.Microseconds()) / 20000
    }

    redMesh := makeMesh(fretColor(0))
    greenMesh := makeMesh(fretColor(1))
    yellowMesh := makeMesh(fretColor(2))
    blueMesh := makeMesh(fretColor(3))
    orangeMesh := makeMesh(fretColor(4))

    neckLength := 800

    // neckMesh := make3dRectangle(70, 5, 300, tetra3d.NewColor(1, 1, 1, 1))
    neckMesh := makePlane(70, neckLength, tetra3d.NewColor(1, 1, 1, 1))
    neckModel := tetra3d.NewModel("Neck", neckMesh)
    neckModel.Color = tetra3d.NewColor(1, 1, 1, 1)
    neckModel.Move(0, -2, 50)
    scene.Root.AddChildren(neckModel)

    guitarSkin := loadSkin()
    neckMesh.MeshPartByMaterialName("Top").Material.Texture = guitarSkin

    for fretI := range song.Frets {
        fretLine := makePlane(1, neckLength, tetra3d.NewColor(0.7, 0.7, 0.7, 0.7))
        fretModel := tetra3d.NewModel("Fret", fretLine)
        fretModel.Move(float32((fretI - 2) * 10), 1, 0)
        neckModel.AddChildren(fretModel)
    }

    particleManager := NewParticleManager(scene)

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

    for _, button := range []*tetra3d.Model{redButton, greenButton, yellowButton, blueButton, orangeButton} {
        scene.Root.AddChildren(button)
    }

    lookPosition := tetra3d.NewVector3(0, 5, -60)

    camera := tetra3d.NewCamera(ScreenWidth, ScreenHeight)
    camera.PerspectiveCorrectedTextureMapping = true
    camera.SetFar(800)
    // camera := tetra3d.NewCamera(300, 300)
    camera.SetFieldOfView(30)
    // camera.SetLocalPosition(0, 10, 500)
    camera.Move(0, 55, 145)
    camera.RenderDepth = true
    // camera.DepthMargin = 0.10
    // camera.RenderNormals = true
    // camera.Rotate(3.5, 0, 0, -0.8)

    camera.SetLocalRotation(tetra3d.NewMatrix4LookAt(lookPosition, camera.WorldPosition(), tetra3d.NewVector3(0, 1, 0)))

    // camera.Node.Move(tetra3d.NewVector3(0, 0, -10))
    // camera.SetLocalRotation(tetra3d.NewMatrix4Rotate(0, 0, 0, 2))

    scene.Root.AddChildren(camera)

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
                // sustainMesh := make3dRectangle(4, 0.1, timeToZ(note.End - note.Start), fretColor(fretI))
                sustainMesh := makePlane(3, int(timeToZ(note.End - note.Start)), fretColor(fretI))
                sustainModel := tetra3d.NewModel("Sustain", sustainMesh)
                sustainModel.Color = tetra3d.NewColor(1, 1, 1, 1)
                sustainModel.Move(0, 2, 0)
                noteModel.SustainModel = sustainModel
                // sustainModel.Move(float32(xPos), 0, float32(-note.Start.Microseconds()/20000) - (float32(note.End.Microseconds() - note.Start.Microseconds()) / 40000))
                model.AddChildren(sustainModel)
            }

            notes = append(notes, noteModel)
        }
    }

    engine.PushDrawer(func(screen *ebiten.Image) {
        engine.DrawSong3d(screen, song, scene, camera)
        // drawSong(screen, song, engine.Font)
    })
    defer engine.PopDrawer()

    var counter uint64

    song.Update(input, particleManager)
    for !song.Finished() {
        counter += 1

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

        // these keys are mainly for debugging
        moved := false
        var move tetra3d.Vector3

        keys = inpututil.AppendPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyQ:
                    move = tetra3d.NewVector3(-1, 0, 0)
                    moved = true
                case ebiten.KeyW:
                    move = tetra3d.NewVector3(1, 0, 0)
                    moved = true
                case ebiten.KeyA:
                    move = tetra3d.NewVector3(0, -1, 0)
                    moved = true
                case ebiten.KeyS:
                    move = tetra3d.NewVector3(0, 1, 0)
                    moved = true
                case ebiten.KeyZ:
                    move = tetra3d.NewVector3(0, 0, -1)
                    moved = true
                case ebiten.KeyX:
                    move = tetra3d.NewVector3(0, 0, 1)
                    moved = true

                case ebiten.KeyE:
                    if camera.FieldOfView() < 179 {
                        camera.SetFieldOfView(camera.FieldOfView() + 1)
                    }
                case ebiten.KeyR:
                    if camera.FieldOfView() > 1 {
                        camera.SetFieldOfView(camera.FieldOfView() - 1)
                    }
            }
        }

        if moved {
            camera.MoveVec(move)
            camera.SetLocalRotation(tetra3d.NewMatrix4LookAt(lookPosition, camera.WorldPosition(), tetra3d.NewVector3(0, 1, 0)))
        }

        delta := time.Since(song.StartTime)

        song.Update(input, particleManager)

        // log.Printf("Notes: %v", len(notes))
        if counter % 2 == 0 {
            notesOut := make([]NoteModel, 0, len(notes))

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
                            alpha = min(1, float32(time.Second * 6) / float32(elapsed))
                        }

                        noteModel.Model.Color.A = alpha

                        noteModel.Model.SetWorldPosition(x, y, timeToZ(-elapsed))
                    }

                    notesOut = append(notesOut, noteModel)
                }

                // noteModel.Move(0, 0, 0.2)
            }

            notes = notesOut

            // model.SetLocalRotation(model.LocalRotation().Rotated(0.5, 0.2, 0.5, 0.02))

            for range 2 {
                particleManager.Update()
            }

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
            case "song.ogg", "song.mp3", "song.opus": hasSong = true
            case "guitar.ogg", "guitar.mp3", "guitar.opus": hasGuitar = true
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
    // camera.DrawDebugWireframe(screen, scene.Root, tetra3d.NewColor(0, 1, 0, 1))
    // camera.DrawDebugDrawOrder(screen, scene.Root, 1, tetra3d.NewColor(1, 0, 0, 1))
    // screen.DrawImage(camera.DepthTexture(), nil)

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

    delta := min(song.SongLength, time.Since(song.StartTime))

    face := &text.GoTextFace{
        Source: engine.Font,
        Size: 24,
    }

    var textOptions text.DrawOptions
    textOptions.GeoM.Translate(ScreenWidth - 250, 10)
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

    textOptions.GeoM.Reset()
    textOptions.GeoM.Translate(10, 10)
    text.Draw(screen, fmt.Sprintf("FPS: %0.2f", ebiten.ActualFPS()), face, &textOptions)

    textOptions.GeoM.Translate(0, 20)
    position := camera.WorldPosition()
    text.Draw(screen, fmt.Sprintf("Camera: X: %v Y: %v Z: %v Fov: %v", position.X, position.Y, position.Z, camera.FieldOfView()), face, &textOptions)

    if song.LyricBatch < len(song.LyricBatches) {
        batch := song.LyricBatches[song.LyricBatch]
        // log.Printf("Batch start: %v, end: %v, delta: %v", batch.StartTime(), batch.EndTime(), delta)
        // log.Printf("%v %v", batch.StartTime() > delta - time.Millisecond * 500, batch.EndTime() < delta + time.Millisecond * 500)
        if delta > batch.StartTime() - time.Millisecond * 500 && delta < batch.EndTime() + time.Millisecond * 500 {
            var totalLyrics string
            for _, lyric := range batch {
                totalLyrics += lyric.Text + " "
            }
            if totalLyrics != "" {
                var lyricOptions text.DrawOptions
                face = &text.GoTextFace{
                    Source: engine.Font,
                    Size: 40,
                }
                // FIXME: draw already played lyrics in a different color
                width, _ := text.Measure(totalLyrics, face, 0)
                lyricOptions.GeoM.Translate(ScreenWidth / 2 - width / 2, 10)
                text.Draw(screen, totalLyrics, face, &lyricOptions)
            }

            nextBatch := song.LyricBatch + 1
            if nextBatch < len(song.LyricBatches) {
                next := song.LyricBatches[nextBatch]

                if next.StartTime() - batch.EndTime() < time.Second {
                    var nextLyrics string
                    for _, lyric := range next {
                        nextLyrics += lyric.Text + " "
                    }
                    if nextLyrics != "" {
                        smallFace := text.GoTextFace{
                            Source: engine.Font,
                            Size: 20,
                        }
                        var lyricOptions text.DrawOptions
                        width, _ := text.Measure(nextLyrics, &smallFace, 0)
                        lyricOptions.GeoM.Reset()
                        lyricOptions.GeoM.Translate(ScreenWidth / 2 - width / 2, 10 + 40 + 10)
                        text.Draw(screen, nextLyrics, &smallFace, &lyricOptions)
                    }
                }
            }
        }
    }

    if delta < time.Second * 2 && song.SongInfo.Name != "" {
        textOptions.GeoM.Translate(0, 20)
        face = &text.GoTextFace{
            Source: engine.Font,
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

    // run the engine at a higher rate to catch more input
    ticks := 120

    ebiten.SetTPS(ticks)
    ebiten.SetWindowSize(ScreenWidth, ScreenHeight)
    ebiten.SetWindowTitle("Rhythm Game")
    ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

    audioContext := audio.NewContext(44100)

    engine, err := MakeEngine(audioContext, path, ticks)

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
