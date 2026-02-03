package main

import (
    "os"
    "fmt"

    "github.com/kazzmir/rhythm/game/chart"
)

func main(){
    if len(os.Args) < 2 {
        fmt.Println("Usage: run <chart_file>")
        return
    }

    chartFile := os.Args[1]
    file, err := os.Open(chartFile)
    if err != nil {
        fmt.Printf("Error opening file: %v\n", err)
        return
    }
    defer file.Close()

    chart.ParseChart(file)
}
