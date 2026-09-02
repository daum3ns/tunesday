package ceremony

import (
	"fmt"
	"math/rand"
)

var adjectives = []string{
	"Glitchy", "Quantum", "Radioactive", "Cosmic", "Sneaky", "Turbo",
	"Phantom", "Binary", "Segfault", "Deadlocked", "Wobbly", "Fuzzy",
	"Neon", "Vintage", "Overdriven", "Unreleased", "Bootleg", "Acid",
	"Crystal", "Midnight", "Hyper", "Rogue", "Silent", "Loud",
	"Feral", "Infinite", "Lo-Fi", "Hi-Fi", "Broken", "Haunted",
	"Melting", "Frozen", "Electric", "Analog", "Digital", "Spectral",
	"Wandering", "Screaming", "Whispering", "Nuclear", "Orbital",
}

var nouns = []string{
	"Synthesizer", "Semaphore", "Waveform", "Coroutine", "Oscillator",
	"Equalizer", "Firewall", "Arpeggiator", "Sequencer", "Amplifier",
	"Harmonic", "Pipeline", "Reverb", "Delay", "Distortion", "Crossover",
	"Autotune", "Sampler", "Drumkit", "Vinyl", "Cassette", "Minidisc",
	"Baudrate", "Pixel", "Kernel", "Daemon", "Registry", "Cache",
	"Compiler", "Linker", "Stack", "Heap", "Pointer", "Bitrate",
	"Fidelity", "Tremolo", "Vibrato", "Glissando", "Pickup", "Headphone",
	"Subwoofer", "Tweeter", "Fader", "Panner", "Clipping", "Nyquist",
}

// NewAlias picks a random adjective+noun combo avoiding used names.
// If every combo is exhausted it falls back to a numbered name.
func NewAlias(used map[string]struct{}) string {
	for attempt := 0; attempt < 100; attempt++ {
		name := fmt.Sprintf("%s %s",
			adjectives[rand.Intn(len(adjectives))],
			nouns[rand.Intn(len(nouns))],
		)
		if _, taken := used[name]; !taken {
			return name
		}
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("Mystery Listener %d", i)
		if _, taken := used[name]; !taken {
			return name
		}
	}
}
