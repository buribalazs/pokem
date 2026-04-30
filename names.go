package main

import "math/rand"

var (
	adjectives = []string{"Red", "Blue", "Gray", "Pink", "Gold", "Jade", "Bold", "Cool", "Fast", "Wild", "Dark", "Lime", "Teal", "Rust"}
	nouns      = []string{"Fox", "Owl", "Bear", "Cat", "Bat", "Bee", "Elk", "Jay", "Ram", "Yak", "Koi", "Wolf", "Hawk", "Lynx"}
)

func genName(taken string) string {
	for {
		name := adjectives[rand.Intn(len(adjectives))] + nouns[rand.Intn(len(nouns))]
		if name != taken {
			return name
		}
	}
}
