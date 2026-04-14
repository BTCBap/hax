package ur

// Bytewords encoding for BC-UR (Blockchain Commons Uniform Resources).
// Each byte maps to a 4-letter word. "Minimal" encoding uses only
// the first and last letters of each word.
// Reference: https://github.com/BlockchainCommons/Research/blob/master/papers/bcr-2020-012-bytewords.md

var bytewordsTable = [256]string{
	"able", "acid", "also", "apex", "aqua", "arch", "atom", "aunt",
	"away", "axis", "back", "bald", "barn", "belt", "beta", "bias",
	"blue", "body", "brag", "brew", "bulb", "buzz", "calm", "cash",
	"cats", "chef", "city", "claw", "code", "cola", "cook", "cost",
	"crux", "curl", "cusp", "cyan", "dark", "data", "days", "deli",
	"dice", "diet", "door", "down", "draw", "drop", "drum", "dull",
	"duty", "each", "easy", "echo", "edge", "epic", "even", "exam",
	"exit", "eyes", "fact", "fair", "fern", "figs", "film", "fish",
	"fizz", "flap", "flew", "flux", "foxy", "free", "frog", "fuel",
	"fund", "gala", "game", "gear", "gems", "gift", "girl", "glow",
	"good", "gray", "grim", "guru", "gush", "gyro", "half", "hang",
	"hard", "hawk", "heat", "help", "high", "hill", "holy", "hope",
	"horn", "huts", "iced", "idea", "idle", "inch", "inky", "into",
	"iris", "iron", "item", "jade", "jazz", "join", "jolt", "jowl",
	"judo", "jugs", "jump", "junk", "jury", "keep", "keno", "kept",
	"keys", "kick", "kiln", "king", "kite", "kiwi", "knob", "lamb",
	"lava", "lazy", "leaf", "legs", "liar", "limp", "lion", "list",
	"logo", "loud", "love", "luau", "luck", "lung", "main", "many",
	"math", "maze", "memo", "menu", "meow", "mild", "mint", "miss",
	"monk", "nail", "navy", "need", "news", "next", "noon", "note",
	"numb", "obey", "oboe", "omit", "onyx", "open", "oval", "owls",
	"paid", "part", "peck", "play", "plus", "poem", "pool", "pose",
	"puff", "puma", "purr", "quad", "quiz", "race", "ramp", "real",
	"redo", "rich", "road", "rock", "roof", "ruby", "ruin", "runs",
	"rust", "safe", "saga", "scar", "sets", "silk", "skew", "slot",
	"soap", "solo", "song", "stub", "surf", "swan", "taco", "task",
	"taxi", "tent", "tied", "time", "tiny", "toil", "tomb", "toys",
	"trip", "tuna", "twin", "ugly", "undo", "unit", "urge", "user",
	"vast", "very", "veto", "vial", "vibe", "view", "visa", "void",
	"vows", "wall", "wand", "warm", "wasp", "wave", "waxy", "webs",
	"what", "when", "whiz", "wolf", "work", "yank", "yawn", "yell",
	"yoga", "yurt", "zaps", "zero", "zest", "zinc", "zone", "zoom",
}

// EncodeMinimal encodes data as minimal bytewords (first+last letter of each word).
// This is the encoding format used inside UR strings.
func EncodeMinimal(data []byte) string {
	result := make([]byte, len(data)*2)
	for i, b := range data {
		word := bytewordsTable[b]
		result[i*2] = word[0]
		result[i*2+1] = word[len(word)-1]
	}
	return string(result)
}
