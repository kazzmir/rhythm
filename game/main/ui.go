package main

import (
    "slices"
    "cmp"
    "os"
    "fmt"
    "time"
    "strings"
    "image/color"
    "math/rand/v2"
    "path/filepath"
    "context"

    "github.com/kazzmir/rhythm/lib/coroutine"
    "github.com/kazzmir/rhythm/lib/colorconv"

    "github.com/ebitenui/ebitenui"
    "github.com/hajimehoshi/ebiten/v2/text/v2"
    "github.com/ebitenui/ebitenui/widget"
    ui_image "github.com/ebitenui/ebitenui/image"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
)

func translucent(c color.Color, alpha int) color.NRGBA {
    r, g, b, _ := c.RGBA()
    return color.NRGBA{
        R: uint8(r >> 8),
        G: uint8(g >> 8),
        B: uint8(b >> 8),
        A: uint8(alpha),
    }
}

func makeButton(text string, tface text.Face, maxWidth int, onClick func(args *widget.ButtonClickedEventArgs)) *widget.Button {
    baseColor := color.NRGBA{R: 100, G: 160, B: 210, A: 255}
    borderColor := color.NRGBA{R: 250, G: 250, B: 250, A: 100}
    alpha := 120
    return widget.NewButton(
        widget.ButtonOpts.WidgetOpts(
            widget.WidgetOpts.LayoutData(widget.GridLayoutData{
                MaxWidth: maxWidth,
            }),
        ),
        widget.ButtonOpts.TextPadding(&widget.Insets{Top: 2, Bottom: 2, Left: 5, Right: 5}),
        widget.ButtonOpts.Image(&widget.ButtonImage{
            Idle: ui_image.NewBorderedNineSliceColor(translucent(darkenColor(baseColor, 0.4), alpha), borderColor, 1),
            Hover: ui_image.NewBorderedNineSliceColor(translucent(baseColor, alpha), borderColor, 1),
            Pressed: ui_image.NewBorderedNineSliceColor(translucent(brightenColor(baseColor, 0.4), alpha), borderColor, 1),
        }),
        widget.ButtonOpts.Text(text, &tface, &widget.ButtonTextColor{
            Idle: color.White,
            Hover: color.White,
            Pressed: color.White,
            Disabled: color.Gray{Y: 128},
        }),
        widget.ButtonOpts.ClickedHandler(onClick),
    )
}

func chooseSong(yield coroutine.YieldFunc, engine *Engine, background *Background, face *text.GoTextFace) string {
    chosen := false

    var tface text.Face = face

    song := ""

    rootContainer := widget.NewContainer(
        widget.ContainerOpts.Layout(widget.NewRowLayout(
            widget.RowLayoutOpts.Direction(widget.DirectionVertical),
            widget.RowLayoutOpts.Spacing(12),
            widget.RowLayoutOpts.Padding(&widget.Insets{Top: 10, Left: 10, Right: 10}),
        )),
    )

    songPaths := scanSongs(".", 0)

    songPaths = slices.SortedFunc(slices.Values(songPaths), func(a, b string) int {
        ax := filepath.Base(strings.ToLower(a))
        bx := filepath.Base(strings.ToLower(b))
        return cmp.Compare(ax, bx)
    })

    songContainer := widget.NewContainer(
        widget.ContainerOpts.Layout(widget.NewRowLayout(
            widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
            widget.RowLayoutOpts.Spacing(12),
        )),
    )

    albumImage := ebiten.NewImage(1, 1)
    albumGraphic := widget.NewGraphic(
        widget.GraphicOpts.Image(albumImage),
    )

    mainQuit, mainCancel := context.WithCancel(context.Background())
    defer mainCancel()

    playSongQuit, playSongCancel := context.WithCancel(mainQuit)
    defer playSongCancel()

    songList := widget.NewList(
        widget.ListOpts.EntryFontFace(&tface),
        widget.ListOpts.SliderParams(&widget.SliderParams{
            TrackImage: &widget.SliderTrackImage{
                Idle: ui_image.NewNineSliceColor(color.NRGBA{R: 150, G: 150, B: 150, A: 255}),
                Hover: ui_image.NewNineSliceColor(color.NRGBA{R: 170, G: 170, B: 170, A: 255}),
            },
            HandleImage: &widget.ButtonImage{
                Idle: ui_image.NewNineSliceColor(color.NRGBA{R: 200, G: 200, B: 200, A: 255}),
                Hover: ui_image.NewNineSliceColor(color.NRGBA{R: 220, G: 220, B: 220, A: 255}),
                Pressed: ui_image.NewNineSliceColor(color.NRGBA{R: 180, G: 180, B: 180, A: 255}),
            },
        }),
        widget.ListOpts.HideHorizontalSlider(),
        widget.ListOpts.ContainerOpts(widget.ContainerOpts.WidgetOpts(
            widget.WidgetOpts.LayoutData(widget.RowLayoutData{
                MaxHeight: ScreenHeight - 20,
            }),
            widget.WidgetOpts.MinSize(0, 200),
        )),
        widget.ListOpts.SelectFocus(),
        widget.ListOpts.DisableDefaultKeys(true),
        widget.ListOpts.EntryLabelFunc(
            func (e any) string {
                name := e.(string)
                return filepath.Base(name)
            },
        ),
        widget.ListOpts.EntrySelectedHandler(func (args *widget.ListEntrySelectedEventArgs) {
            entry := args.Entry.(string)
            song = entry

            newImage := loadAlbumImage(os.DirFS(song))

            oldAlbum := albumGraphic
            albumGraphic = widget.NewGraphic(
                widget.GraphicOpts.Image(newImage),
            )
            songContainer.ReplaceChild(oldAlbum, albumGraphic)

            playSongCancel()
            playSongQuit, playSongCancel = context.WithCancel(mainQuit)

            localQuit := playSongQuit
            go func() {
                select {
                    case <-time.After(200 * time.Millisecond):
                    case <-localQuit.Done():
                        return
                }

                songPlayer, _, _, err := loadSong(engine.AudioContext, os.DirFS(song))
                if err != nil {
                    return
                }
                guitarPlayer, _, err := loadGuitarSong(engine.AudioContext, os.DirFS(song))
                if err != nil {
                    return
                }

                songPlayer.Play()
                guitarPlayer.Play()

                select {
                    case <-localQuit.Done():
                        songPlayer.Pause()
                        guitarPlayer.Pause()
                }

            }()
        }),
        widget.ListOpts.EntryColor(&widget.ListEntryColor{
            Selected: color.NRGBA{R: 100, G: 150, B: 200, A: 255},
            Unselected: color.NRGBA{R: 50, G: 50, B: 50, A: 255},
        }),
        widget.ListOpts.ScrollContainerImage(&widget.ScrollContainerImage{
            Idle: ui_image.NewNineSliceColor(color.NRGBA{R: 220, G: 220, B: 220, A: 80}),
            Disabled: ui_image.NewNineSliceColor(color.NRGBA{R: 180, G: 180, B: 180, A: 255}),
            Mask: ui_image.NewNineSliceColor(color.NRGBA{R: 32, G: 32, B: 32, A: 255}),
        }),
    )

    for _, songPath := range songPaths {
        songList.AddEntry(songPath)
    }

    /*
    playButton := makeButton("Play Selected Song", tface, 200, func (args *widget.ButtonClickedEventArgs) {
        if song != "" {
            chosen = true
        }
    })
    */

    backButton := makeButton("Back", tface, 200, func (args *widget.ButtonClickedEventArgs) {
        song = ""
        chosen = true
    })

    songContainer.AddChild(songList)
    songContainer.AddChild(albumGraphic)

    rootContainer.AddChild(songContainer)
    // rootContainer.AddChild(playButton)
    rootContainer.AddChild(backButton)

    songList.Focus(true)

    ui := ebitenui.UI{
        Container: rootContainer,
    }

    engine.PushDrawer(func(screen *ebiten.Image) {
        background.Draw(screen)
        ui.Draw(screen)
    })
    defer engine.PopDrawer()

    getRepeatDelay := func(count uint64) int {
        switch {
            case count >= 10: return 25
            case count >= 5: return 60
            case count >= 2: return 100
        }

        return 250
    }

    var keyDownTime time.Time
    var keyDownCount uint64 = 0
    var keyUpTime time.Time
    var keyUpCount uint64 = 0

    for !chosen {

        keys := inpututil.AppendPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyDown:
                    if !keyDownTime.IsZero() && time.Since(keyDownTime) > time.Duration(getRepeatDelay(keyDownCount)) * time.Millisecond {
                        songList.FocusNext()
                        keyDownCount += 1
                        keyDownTime = time.Now()
                    }
                case ebiten.KeyUp:
                    if !keyUpTime.IsZero() && time.Since(keyUpTime) > time.Duration(getRepeatDelay(keyUpCount)) * time.Millisecond {
                        songList.FocusPrevious()
                        keyUpCount += 1
                        keyUpTime = time.Now()
                    }
            }
        }


        keys = inpututil.AppendJustPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyEscape, ebiten.KeyCapsLock:
                    return ""
                case ebiten.KeyEnter:
                    if song != "" {
                        return song
                    }
                case ebiten.KeyDown:
                    songList.FocusNext()
                    keyDownTime = time.Now()
                case ebiten.KeyUp:
                    songList.FocusPrevious()
                    keyUpTime = time.Now()
                case ebiten.KeyPageDown:
                    for range 10 {
                        songList.FocusNext()
                    }
                case ebiten.KeyPageUp:
                    for range 10 {
                        songList.FocusPrevious()
                    }
            }
        }

        keys = inpututil.AppendJustReleasedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyDown:
                    keyDownTime = time.Time{}
                    keyDownCount = 0
                case ebiten.KeyUp:
                    keyUpTime = time.Time{}
                    keyUpCount = 0
            }
        }

        background.Update()
        ui.Update()

        if yield() != nil {
            return ""
        }
    }

    return song
}

type ColorHSV struct {
    H, S, V float64 // h 0-360, s 0-1, v 0-1

    R, G, B uint8
    converted bool
}

func (c *ColorHSV) Update(target ColorHSV) {
    changed := false

    hStep := 0.5
    sStep := 0.001
    vStep := 0.001

    update := func(source *float64, target float64, step float64) {
        if *source <= target - step {
            *source += step
            changed = true
        } else if *source >= target + step {
            *source -= step
            changed = true
        } else {
            *source = target
        }
    }

    update(&c.H, target.H, hStep)
    update(&c.S, target.S, sStep)
    update(&c.V, target.V, vStep)

    if changed {
        c.converted = false
    }
}

func (c *ColorHSV) ToNRGBA() color.NRGBA {
    if !c.converted {
        col, err := colorconv.HSVToColor(c.H, c.S, c.V)
        if err == nil {
            r, g, b, _ := col.RGBA()
            c.R = uint8(r >> 8)
            c.G = uint8(g >> 8)
            c.B = uint8(b >> 8)
            c.converted = true
        }
    }

    return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
}

type Background struct {
    C1 ColorHSV
    C2 ColorHSV
    C3 ColorHSV
    C4 ColorHSV

    ChangeC1 ColorHSV
    ChangeC2 ColorHSV
    ChangeC3 ColorHSV
    ChangeC4 ColorHSV

    Source *ebiten.Image
    counter uint64
}

func makeRandomColor() ColorHSV {
    return ColorHSV{
        H: rand.Float64() * 360.0,
        S: 0.2 + rand.Float64() * 0.4,
        V: 0.2 + rand.Float64() * 0.4,
    }
}

func MakeBackground() *Background {
    white := ebiten.NewImage(1, 1)
    white.Fill(color.White)

    return &Background{
        C1: makeRandomColor(),
        C2: makeRandomColor(),
        C3: makeRandomColor(),
        C4: makeRandomColor(),
        ChangeC1: makeRandomColor(),
        ChangeC2: makeRandomColor(),
        ChangeC3: makeRandomColor(),
        ChangeC4: makeRandomColor(),
        Source: white,
    }
}

func (background *Background) Update() {
    background.counter += 1
    if background.counter % 600 == 0 {
        background.ChangeC1 = makeRandomColor()
        background.ChangeC2 = makeRandomColor()
        background.ChangeC3 = makeRandomColor()
        background.ChangeC4 = makeRandomColor()
    }

    background.C1.Update(background.ChangeC1)
    background.C2.Update(background.ChangeC2)
    background.C3.Update(background.ChangeC3)
    background.C4.Update(background.ChangeC4)
}

func (background *Background) Draw(screen *ebiten.Image) {
    vertices := []ebiten.Vertex{
        ebiten.Vertex{
            DstX: 0,
            DstY: 0,
            SrcX: 0,
            SrcY: 0,
            ColorR: float32(background.C1.ToNRGBA().R) / 255.0,
            ColorG: float32(background.C1.ToNRGBA().G) / 255.0,
            ColorB: float32(background.C1.ToNRGBA().B) / 255.0,
            ColorA: 1.0,
        },
        ebiten.Vertex{
            DstX: float32(ScreenWidth),
            DstY: 0,
            SrcX: 0,
            SrcY: 0,
            ColorR: float32(background.C2.ToNRGBA().R) / 255.0,
            ColorG: float32(background.C2.ToNRGBA().G) / 255.0,
            ColorB: float32(background.C2.ToNRGBA().B) / 255.0,
            ColorA: 1.0,
        },
        ebiten.Vertex{
            DstX: float32(ScreenWidth),
            DstY: float32(ScreenHeight),
            SrcX: 0,
            SrcY: 0,
            ColorR: float32(background.C3.ToNRGBA().R) / 255.0,
            ColorG: float32(background.C3.ToNRGBA().G) / 255.0,
            ColorB: float32(background.C3.ToNRGBA().B) / 255.0,
            ColorA: 1.0,
        },
        ebiten.Vertex{
            DstX: 0,
            DstY: float32(ScreenHeight),
            SrcX: 0,
            SrcY: 0,
            ColorR: float32(background.C4.ToNRGBA().R) / 255.0,
            ColorG: float32(background.C4.ToNRGBA().G) / 255.0,
            ColorB: float32(background.C4.ToNRGBA().B) / 255.0,
            ColorA: 1.0,
        },
    }

    screen.DrawTriangles(vertices, []uint16{0, 1, 2, 2, 3, 0}, background.Source, nil)
}

func doSettingsMenu(yield coroutine.YieldFunc, engine *Engine, background *Background, face *text.GoTextFace) {
    quit := false

    var tface text.Face = face

    rootContainer := widget.NewContainer(
        widget.ContainerOpts.Layout(widget.NewGridLayout(
            widget.GridLayoutOpts.Columns(1),
            widget.GridLayoutOpts.DefaultStretch(true, false),
            widget.GridLayoutOpts.Spacing(0, 10),
            widget.GridLayoutOpts.Padding(&widget.Insets{Top: 80, Left: 20, Right: 10, Bottom: 10}),
        )),
    )

    var fullscreenButton *widget.Button

    var makeFullscreenButton func() *widget.Button

    maxButtonWidth := 250

    makeFullscreenButton = func() *widget.Button {
        oldButton := fullscreenButton
        if ebiten.IsFullscreen() {
            fullscreenButton = makeButton("Windowed Mode", tface, maxButtonWidth, func (args *widget.ButtonClickedEventArgs) {
                ebiten.SetFullscreen(false)
                makeFullscreenButton()
            })
        } else {
            fullscreenButton = makeButton("Fullscreen", tface, maxButtonWidth, func (args *widget.ButtonClickedEventArgs) {
                ebiten.SetFullscreen(true)
                makeFullscreenButton()
            })
        }
        rootContainer.ReplaceChild(oldButton, fullscreenButton)
        fullscreenButton.Focus(true)
        return fullscreenButton
    }

    rootContainer.AddChild(makeFullscreenButton())

    rootContainer.AddChild(makeButton("Back", tface, maxButtonWidth, func (args *widget.ButtonClickedEventArgs) {
        quit = true
    }))

    ui := ebitenui.UI{
        Container: rootContainer,
    }

    engine.PushDrawer(func(screen *ebiten.Image) {
        background.Draw(screen)
        ui.Draw(screen)
    })
    defer engine.PopDrawer()

    for !quit {
        keys := inpututil.AppendJustPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyEscape, ebiten.KeyCapsLock:
                    quit = true
                case ebiten.KeyDown:
                    ui.ChangeFocus(widget.FOCUS_NEXT)
                case ebiten.KeyUp:
                    ui.ChangeFocus(widget.FOCUS_PREVIOUS)
            }
        }

        background.Update()
        ui.Update()
        if yield() != nil {
            return
        }
    }
}

func setupSong(yield coroutine.YieldFunc, engine *Engine, songPath string, face *text.GoTextFace, background *Background) (SongSettings, bool) {
    var settings SongSettings
    settings.Difficulty = "medium"

    var tface text.Face = face

    quit := false
    canceled := false

    var ui ebitenui.UI

    var buildRootContainer func() *widget.Container

    buildDifficultyContainer := func() *widget.Container {
        container := widget.NewContainer(
            widget.ContainerOpts.Layout(widget.NewGridLayout(
                widget.GridLayoutOpts.Columns(1),
                widget.GridLayoutOpts.DefaultStretch(true, false),
                widget.GridLayoutOpts.Spacing(10, 0),
                widget.GridLayoutOpts.Padding(&widget.Insets{Top: 10, Left: 50, Right: 50, Bottom: 50}),
            )),
        )

        for _, difficulty := range []string{"expert", "hard", "medium", "easy"} {
            container.AddChild(makeButton(difficulty, tface, 200, func (args *widget.ButtonClickedEventArgs) {
                settings.Difficulty = difficulty
                ui.Container = buildRootContainer()
            }))
        }

        return container
    }

    buildRootContainer = func() *widget.Container {
        rootContainer := widget.NewContainer(
            widget.ContainerOpts.Layout(widget.NewGridLayout(
                widget.GridLayoutOpts.Columns(1),
                widget.GridLayoutOpts.DefaultStretch(true, false),
                widget.GridLayoutOpts.Spacing(0, 10),
                widget.GridLayoutOpts.Padding(&widget.Insets{Top: 80, Left: 20, Right: 10, Bottom: 10}),
            )),
        )

        rootContainer.AddChild(widget.NewLabel(
            widget.LabelOpts.Text(fmt.Sprintf("Difficulty: %v", settings.Difficulty), &tface, &widget.LabelColor{
                Idle: color.White,
                Disabled: color.Gray{Y: 128},
            }),
        ))

        readyButton := makeButton("Ready", tface, 200, func (args *widget.ButtonClickedEventArgs) {
            quit = true
        })

        rootContainer.AddChild(readyButton)

        // readyButton.Focus(true)

        rootContainer.AddChild(makeButton("Difficulty", tface, 200, func (args *widget.ButtonClickedEventArgs) {
            ui.Container = buildDifficultyContainer()
        }))

        return rootContainer
    }

    ui = ebitenui.UI{
        Container: buildRootContainer(),
    }

    engine.PushDrawer(func(screen *ebiten.Image) {
        background.Draw(screen)
        ui.Draw(screen)
    })
    defer engine.PopDrawer()

    for !quit {
        keys := inpututil.AppendJustPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyEscape, ebiten.KeyCapsLock:
                    quit = true
                    canceled = true
                case ebiten.KeyDown:
                    ui.ChangeFocus(widget.FOCUS_NEXT)
                case ebiten.KeyUp:
                    ui.ChangeFocus(widget.FOCUS_PREVIOUS)
            }
        }

        ui.Update()

        if yield() != nil {
            break
        }
    }

    return settings, canceled
}

func mainMenu(engine *Engine, yield coroutine.YieldFunc) error {
    quit := false

    face := &text.GoTextFace{
        Source: engine.Font,
        Size: 28,
    }
    var tface text.Face = face

    background := MakeBackground()

    /*
    rootContainer := widget.NewContainer(
        widget.ContainerOpts.Layout(widget.NewRowLayout(
            widget.RowLayoutOpts.Direction(widget.DirectionVertical),
            widget.RowLayoutOpts.Spacing(12),
            widget.RowLayoutOpts.Padding(&widget.Insets{Top: 10, Left: 10, Right: 10}),
        )),
    )
    */

    rootContainer := widget.NewContainer(
        widget.ContainerOpts.Layout(widget.NewGridLayout(
            widget.GridLayoutOpts.Columns(1),
            widget.GridLayoutOpts.DefaultStretch(true, false),
            widget.GridLayoutOpts.Spacing(0, 10),
            widget.GridLayoutOpts.Padding(&widget.Insets{Top: 80, Left: 20, Right: 10, Bottom: 10}),
        )),
    )

    selectButton := makeButton("Select Song", tface, 200, func (args *widget.ButtonClickedEventArgs) {
        selectedSong := chooseSong(yield, engine, background, face)
        if selectedSong != "" {

            yield()
            setup, canceled := setupSong(yield, engine, selectedSong, face, background)
            // yield()

            if !canceled {
                playSong(yield, engine, selectedSong, setup)
            } else {
                yield()
            }
        }
    })

    selectButton.Focus(true)
    rootContainer.AddChild(selectButton)

    rootContainer.AddChild(makeButton("Settings", tface, 200, func (args *widget.ButtonClickedEventArgs) {
        doSettingsMenu(yield, engine, background, face)
    }))

    rootContainer.AddChild(makeButton("Quit", tface, 200, func (args *widget.ButtonClickedEventArgs) {
        quit = true
    }))

    ui := ebitenui.UI{
        Container: rootContainer,
    }

    engine.PushDrawer(func(screen *ebiten.Image) {
        background.Draw(screen)
        ui.Draw(screen)
    })
    defer engine.PopDrawer()

    for !quit {

        keys := inpututil.AppendJustPressedKeys(nil)
        for _, key := range keys {
            switch key {
                case ebiten.KeyEscape, ebiten.KeyCapsLock:
                    quit = true
                case ebiten.KeyDown:
                    ui.ChangeFocus(widget.FOCUS_NEXT)
                case ebiten.KeyUp:
                    ui.ChangeFocus(widget.FOCUS_PREVIOUS)
            }
        }

        background.Update()
        ui.Update()

        if yield() != nil {
            break
        }
    }

    return nil
}
