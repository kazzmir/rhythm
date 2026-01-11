package data

import (
    "embed"
)

//go:embed flame/*
var FlameFS embed.FS

//go:embed skins/*
var SkinsFS embed.FS
