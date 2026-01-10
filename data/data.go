package data

import (
    "embed"
)

//go:embed flame/*
var FlameFS embed.FS
