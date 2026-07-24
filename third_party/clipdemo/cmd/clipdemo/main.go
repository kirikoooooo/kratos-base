package main

import (
	"context"
	"fmt"
	"os"

	"kratos-demo/third_party/clipdemo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: clipdemo <text> [text...]")
		os.Exit(2)
	}
	scores, err := clipdemo.CompareImageToTexts(context.Background(), clipdemo.Config{BaseURL: os.Getenv("CLIP_BASE_URL"), APIKey: os.Getenv("CLIP_API_KEY")}, os.Getenv("CLIP_IMAGE"), os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for i, score := range scores {
		fmt.Printf("%.6f\t%s\n", score, os.Args[i+1])
	}
}
