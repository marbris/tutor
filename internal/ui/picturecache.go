package ui

// The pictures in memory, kept to a budget.
//
// A picture in hand is the PNG kitty is sent, about 750 KB, against the
// 80 KB JPEG kept on disk. Holding every one gx or gX had ever fetched let a
// long session climb past a gigabyte, so only the most recently used are
// kept. One that was let go is read from disk and converted again the next
// time it's shown (~50 ms, no request). The one on screen is never let go.

// pictureBudget is how many bytes of PNG are kept in memory: some sixty
// pictures, enough for walking a list back and forth after a gX.
const pictureBudget = 48 << 20

type pictureCache struct {
	byKey map[string]*keptPicture
	bytes int
	clock int
	// budget is pictureBudget, smaller in the tests.
	budget int
}

type keptPicture struct {
	pic  *picture
	used int
}

func newPictureCache() *pictureCache {
	return &pictureCache{byKey: map[string]*keptPicture{}, budget: pictureBudget}
}

// peek is a picture, without counting it as used — for drawing.
func (pc *pictureCache) peek(key string) (*picture, bool) {
	k, ok := pc.byKey[key]
	if !ok {
		return nil, false
	}
	return k.pic, true
}

// get is a picture, counted as used.
func (pc *pictureCache) get(key string) (*picture, bool) {
	k, ok := pc.byKey[key]
	if !ok {
		return nil, false
	}
	pc.clock++
	k.used = pc.clock
	return k.pic, true
}

// put files a picture, then lets the least recently used go until the
// budget holds, sparing the one just put and keep (the one on screen).
func (pc *pictureCache) put(key string, pic *picture, keep string) {
	if old, ok := pc.byKey[key]; ok {
		pc.bytes -= len(old.pic.png)
	}
	pc.clock++
	pc.byKey[key] = &keptPicture{pic: pic, used: pc.clock}
	pc.bytes += len(pic.png)
	for pc.bytes > pc.budget {
		oldest, at := "", 0
		for k, kp := range pc.byKey {
			if k == key || k == keep || len(kp.pic.png) == 0 {
				continue
			}
			if oldest == "" || kp.used < at {
				oldest, at = k, kp.used
			}
		}
		if oldest == "" {
			return
		}
		pc.bytes -= len(pc.byKey[oldest].pic.png)
		delete(pc.byKey, oldest)
	}
}
