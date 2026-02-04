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

    use, err := chart.ParseChart(file)
    if err != nil {
        fmt.Printf("Error parsing chart: %v\n", err)
        return
    }

    for _, note := range use.GetNotes("Expert") {
        fmt.Printf("Note: %+v\n", note)

        /*
        if i > 5 {
            break
        }
        */
    }
}
