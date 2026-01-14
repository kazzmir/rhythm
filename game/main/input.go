package main

import (
    "io"
    "encoding/json"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
)

type SerializedGamepadProfile struct {
    GamepadID ebiten.GamepadID `json:"gamepad_id"`
    Name string `json:"name"`
    GreenButton ebiten.GamepadButton `json:"green_button"`
    RedButton ebiten.GamepadButton `json:"red_button"`
    YellowButton ebiten.GamepadButton `json:"yellow_button"`
    BlueButton ebiten.GamepadButton `json:"blue_button"`
    OrangeButton ebiten.GamepadButton `json:"orange_button"`
    StrumUpButton ebiten.GamepadButton `json:"strum_up_button"`
    StrumDownButton ebiten.GamepadButton `json:"strum_down_button"`
}

type InputProfileGamepad struct {
    GamepadID ebiten.GamepadID

    GreenButton ebiten.GamepadButton
    RedButton ebiten.GamepadButton
    YellowButton ebiten.GamepadButton
    BlueButton ebiten.GamepadButton
    OrangeButton ebiten.GamepadButton
    StrumUpButton ebiten.GamepadButton
    StrumDownButton ebiten.GamepadButton
}

func NewInputProfileGamepad(id ebiten.GamepadID) *InputProfileGamepad {
    return &InputProfileGamepad{
        GamepadID: id,
        GreenButton: ebiten.GamepadButton(-1),
        RedButton: ebiten.GamepadButton(-1),
        YellowButton: ebiten.GamepadButton(-1),
        BlueButton: ebiten.GamepadButton(-1),
        OrangeButton: ebiten.GamepadButton(-1),
        StrumUpButton: ebiten.GamepadButton(-1),
        StrumDownButton: ebiten.GamepadButton(-1),
    }
}

func (profile *InputProfileGamepad) Serialize() SerializedGamepadProfile {
    return SerializedGamepadProfile{
        GamepadID: profile.GamepadID,
        Name: ebiten.GamepadName(profile.GamepadID),
        GreenButton: profile.GreenButton,
        RedButton: profile.RedButton,
        YellowButton: profile.YellowButton,
        BlueButton: profile.BlueButton,
        OrangeButton: profile.OrangeButton,
        StrumUpButton: profile.StrumUpButton,
        StrumDownButton: profile.StrumDownButton,
    }
}

func (profile *InputProfileGamepad) SetInput(kind InputAction, button ebiten.GamepadButton) {
    switch kind {
        case InputActionGreen: profile.GreenButton = button
        case InputActionRed: profile.RedButton = button
        case InputActionYellow: profile.YellowButton = button
        case InputActionBlue: profile.BlueButton = button
        case InputActionOrange: profile.OrangeButton = button
        case InputActionStrumUp: profile.StrumUpButton = button
        case InputActionStrumDown: profile.StrumDownButton = button
    }
}

func (profile *InputProfileGamepad) GetInput(kind InputAction) ebiten.GamepadButton {
    switch kind {
        case InputActionGreen: return profile.GreenButton
        case InputActionRed: return profile.RedButton
        case InputActionYellow: return profile.YellowButton
        case InputActionBlue: return profile.BlueButton
        case InputActionOrange: return profile.OrangeButton
        case InputActionStrumUp: return profile.StrumUpButton
        case InputActionStrumDown: return profile.StrumDownButton
    }

    return ebiten.GamepadButton(-1)
}

type InputProfileKeyboard struct {
    GreenButton ebiten.Key `json:"green_button"`
    RedButton ebiten.Key `json:"red_button"`
    YellowButton ebiten.Key `json:"yellow_button"`
    BlueButton ebiten.Key `json:"blue_button"`
    OrangeButton ebiten.Key `json:"orange_button"`
    StrumUpButton ebiten.Key `json:"strum_up_button"`
    StrumDownButton ebiten.Key `json:"strum_down_button"`
}

func (profile *InputProfileKeyboard) SetInput(kind InputAction, key ebiten.Key) {
    switch kind {
        case InputActionGreen: profile.GreenButton = key
        case InputActionRed: profile.RedButton = key
        case InputActionYellow: profile.YellowButton = key
        case InputActionBlue: profile.BlueButton = key
        case InputActionOrange: profile.OrangeButton = key
        case InputActionStrumUp: profile.StrumUpButton = key
        case InputActionStrumDown: profile.StrumDownButton = key
    }
}

func (profile *InputProfileKeyboard) GetInput(kind InputAction) ebiten.Key {
    switch kind {
        case InputActionGreen: return profile.GreenButton
        case InputActionRed: return profile.RedButton
        case InputActionYellow: return profile.YellowButton
        case InputActionBlue: return profile.BlueButton
        case InputActionOrange: return profile.OrangeButton
        case InputActionStrumUp: return profile.StrumUpButton
        case InputActionStrumDown: return profile.StrumDownButton
    }

    return ebiten.Key(-1)
}

func NewInputProfileKeyboard() *InputProfileKeyboard {
    return &InputProfileKeyboard{
        GreenButton: ebiten.Key1,
        RedButton: ebiten.Key2,
        YellowButton: ebiten.Key3,
        BlueButton: ebiten.Key4,
        OrangeButton: ebiten.Key5,
        StrumUpButton: ebiten.KeyUp,
        StrumDownButton: ebiten.KeySpace,
    }
}

/*
type InputProfileInterface interface {
    SetInput(kind InputKind, key ebiten.Key)
}
*/

type UseProfileKind int
const (
    UseProfileKeyboard UseProfileKind = iota
    UseProfileGamepad
)

type InputProfile struct {
    KeyboardProfile *InputProfileKeyboard
    GamepadProfiles map[ebiten.GamepadID]*InputProfileGamepad

    CurrentProfile UseProfileKind
    CurrentGamepadProfile *InputProfileGamepad
}

func NewInputProfile() *InputProfile {
    return &InputProfile{
        KeyboardProfile: NewInputProfileKeyboard(),
        GamepadProfiles: make(map[ebiten.GamepadID]*InputProfileGamepad),
        CurrentProfile: UseProfileKeyboard,
    }
}

func (profile *InputProfile) SetKeyboardProfile(keyboardProfile *InputProfileKeyboard) {
    profile.KeyboardProfile = keyboardProfile
    profile.CurrentProfile = UseProfileKeyboard
}

func (profile *InputProfile) SetGamepadProfile(gamepadProfile *InputProfileGamepad) {
    profile.CurrentGamepadProfile = gamepadProfile
    profile.CurrentProfile = UseProfileGamepad
}

func (profile *InputProfile) GetGamepadProfile(id ebiten.GamepadID) *InputProfileGamepad {
    _, ok := profile.GamepadProfiles[id]
    if !ok {
        profile.GamepadProfiles[id] = NewInputProfileGamepad(id)
    }

    return profile.GamepadProfiles[id]
}

func (profile *InputProfile) IsJustPressed(action InputAction) bool {
    switch profile.CurrentProfile {
        case UseProfileKeyboard:
            key := profile.KeyboardProfile.GetInput(action)
            return inpututil.IsKeyJustPressed(key)
        case UseProfileGamepad:
            button := profile.CurrentGamepadProfile.GetInput(action)
            return inpututil.IsGamepadButtonJustPressed(profile.CurrentGamepadProfile.GamepadID, button)
    }

    return false
}

func (profile *InputProfile) IsJustReleased(action InputAction) bool {
    switch profile.CurrentProfile {
        case UseProfileKeyboard:
            key := profile.KeyboardProfile.GetInput(action)
            return inpututil.IsKeyJustReleased(key)
        case UseProfileGamepad:
            button := profile.CurrentGamepadProfile.GetInput(action)
            return inpututil.IsGamepadButtonJustReleased(profile.CurrentGamepadProfile.GamepadID, button)
    }

    return false
}

type SerializedInputProfile struct {
    KeyboardProfile InputProfileKeyboard `json:"keyboard_profile"`
    GamepadProfiles []SerializedGamepadProfile `json:"gamepad_profiles"`
}

func (profile *InputProfile) Serialize(out io.Writer) error {
    serialized := SerializedInputProfile{
        KeyboardProfile: *profile.KeyboardProfile,
        GamepadProfiles: make([]SerializedGamepadProfile, 0),
    }

    for _, gamepadProfile := range profile.GamepadProfiles {
        serialized.GamepadProfiles = append(serialized.GamepadProfiles, gamepadProfile.Serialize())
    }

    encoder := json.NewEncoder(out)
    return encoder.Encode(&serialized)
}

