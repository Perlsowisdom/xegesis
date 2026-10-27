package verse

// Book is one book of the Bible. Numbers 1–39 are the Old Testament, 40–66 the New Testament and
// 67 onward the deuterocanonical books, so that verse IDs sort in canonical order.
type Book struct {
	Number   int      `json:"number"`
	OSIS     string   `json:"osis"` // OSIS book ID
	Name     string   `json:"name"`
	Chapters int      `json:"chapters"` // most chapters in any supported numbering
	Aliases  []string `json:"-"`
}

var books = []Book{
	{1, "Gen", "Genesis", 50, []string{"ge", "gn"}},
	{2, "Exod", "Exodus", 40, []string{"ex", "exo"}},
	{3, "Lev", "Leviticus", 27, []string{"le", "lv"}},
	{4, "Num", "Numbers", 36, []string{"nu", "nm", "nb"}},
	{5, "Deut", "Deuteronomy", 34, []string{"dt", "de"}},
	{6, "Josh", "Joshua", 24, []string{"jos", "jsh"}},
	{7, "Judg", "Judges", 21, []string{"jdg", "jg", "jdgs"}},
	{8, "Ruth", "Ruth", 4, []string{"rth", "ru"}},
	{9, "1Sam", "1 Samuel", 31, []string{"1sa", "1s"}},
	{10, "2Sam", "2 Samuel", 24, []string{"2sa", "2s"}},
	{11, "1Kgs", "1 Kings", 22, []string{"1ki", "1k", "1kin"}},
	{12, "2Kgs", "2 Kings", 25, []string{"2ki", "2k", "2kin"}},
	{13, "1Chr", "1 Chronicles", 29, []string{"1ch", "1chron"}},
	{14, "2Chr", "2 Chronicles", 36, []string{"2ch", "2chron"}},
	{15, "Ezra", "Ezra", 10, []string{"ezr"}},
	{16, "Neh", "Nehemiah", 13, []string{"ne"}},
	{17, "Esth", "Esther", 10, []string{"es", "est"}},
	{18, "Job", "Job", 42, []string{"jb"}},
	{19, "Ps", "Psalms", 150, []string{"psalm", "psa", "pss", "psm"}},
	{20, "Prov", "Proverbs", 31, []string{"pr", "prv", "pro"}},
	{21, "Eccl", "Ecclesiastes", 12, []string{"ec", "ecc", "qoh"}},
	{22, "Song", "Song of Songs", 8, []string{"so", "sos", "songofsolomon", "canticles", "cant"}},
	{23, "Isa", "Isaiah", 66, []string{"is"}},
	{24, "Jer", "Jeremiah", 52, []string{"je", "jr"}},
	{25, "Lam", "Lamentations", 5, []string{"la"}},
	{26, "Ezek", "Ezekiel", 48, []string{"eze", "ezk"}},
	{27, "Dan", "Daniel", 12, []string{"da", "dn"}},
	{28, "Hos", "Hosea", 14, []string{"ho"}},
	{29, "Joel", "Joel", 4, []string{"jl", "joe"}},
	{30, "Amos", "Amos", 9, []string{"am"}},
	{31, "Obad", "Obadiah", 1, []string{"ob", "oba"}},
	{32, "Jonah", "Jonah", 4, []string{"jon", "jnh"}},
	{33, "Mic", "Micah", 7, []string{"mi"}},
	{34, "Nah", "Nahum", 3, []string{"na"}},
	{35, "Hab", "Habakkuk", 3, []string{"hb"}},
	{36, "Zeph", "Zephaniah", 3, []string{"zep", "zp"}},
	{37, "Hag", "Haggai", 2, []string{"hg"}},
	{38, "Zech", "Zechariah", 14, []string{"zec", "zc"}},
	{39, "Mal", "Malachi", 4, []string{"ml"}},
	{40, "Matt", "Matthew", 28, []string{"mt", "mat"}},
	{41, "Mark", "Mark", 16, []string{"mk", "mar", "mrk"}},
	{42, "Luke", "Luke", 24, []string{"lk", "luk"}},
	{43, "John", "John", 21, []string{"jn", "jhn", "joh"}},
	{44, "Acts", "Acts", 28, []string{"ac", "act"}},
	{45, "Rom", "Romans", 16, []string{"ro", "rm"}},
	{46, "1Cor", "1 Corinthians", 16, []string{"1co"}},
	{47, "2Cor", "2 Corinthians", 13, []string{"2co"}},
	{48, "Gal", "Galatians", 6, []string{"ga"}},
	{49, "Eph", "Ephesians", 6, []string{"ephes"}},
	{50, "Phil", "Philippians", 4, []string{"php", "pp"}},
	{51, "Col", "Colossians", 4, []string{"co"}},
	{52, "1Thess", "1 Thessalonians", 5, []string{"1th", "1thes"}},
	{53, "2Thess", "2 Thessalonians", 3, []string{"2th", "2thes"}},
	{54, "1Tim", "1 Timothy", 6, []string{"1ti", "1tm"}},
	{55, "2Tim", "2 Timothy", 4, []string{"2ti", "2tm"}},
	{56, "Titus", "Titus", 3, []string{"tit", "ti"}},
	{57, "Phlm", "Philemon", 1, []string{"phm", "philem"}},
	{58, "Heb", "Hebrews", 13, []string{"he"}},
	{59, "Jas", "James", 5, []string{"jm", "jam"}},
	{60, "1Pet", "1 Peter", 5, []string{"1pe", "1pt", "1p"}},
	{61, "2Pet", "2 Peter", 3, []string{"2pe", "2pt", "2p"}},
	{62, "1John", "1 John", 5, []string{"1jn", "1jo", "1j"}},
	{63, "2John", "2 John", 1, []string{"2jn", "2jo", "2j"}},
	{64, "3John", "3 John", 1, []string{"3jn", "3jo", "3j"}},
	{65, "Jude", "Jude", 1, []string{"jud", "jd"}},
	{66, "Rev", "Revelation", 22, []string{"re", "rv", "apocalypse", "revelations"}},
	{67, "Tob", "Tobit", 14, []string{"tb"}},
	{68, "Jdt", "Judith", 16, []string{"jth"}},
	{69, "EsthGr", "Esther (Greek)", 16, []string{"addesth", "gkesth"}},
	{70, "Wis", "Wisdom of Solomon", 19, []string{"ws", "wisdom"}},
	{71, "Sir", "Sirach", 51, []string{"ecclesiasticus", "ecclus"}},
	{72, "Bar", "Baruch", 6, nil},
	{73, "EpJer", "Letter of Jeremiah", 1, []string{"letjer"}},
	{74, "PrAzar", "Prayer of Azariah", 1, []string{"songofthree", "sgthree"}},
	{75, "Sus", "Susanna", 1, nil},
	{76, "Bel", "Bel and the Dragon", 1, nil},
	{77, "1Macc", "1 Maccabees", 16, []string{"1ma", "1mac"}},
	{78, "2Macc", "2 Maccabees", 15, []string{"2ma", "2mac"}},
	{79, "3Macc", "3 Maccabees", 7, []string{"3ma", "3mac"}},
	{80, "4Macc", "4 Maccabees", 18, []string{"4ma", "4mac"}},
	{81, "PrMan", "Prayer of Manasseh", 1, []string{"prman", "manasseh"}},
	{82, "1Esd", "1 Esdras", 9, []string{"1es"}},
	{83, "2Esd", "2 Esdras", 16, []string{"2es"}},
	{84, "AddPs", "Psalm 151", 1, []string{"ps151"}},
}

var byName = map[string]*Book{}

func init() {
	for i := range books {
		b := &books[i]
		for _, n := range append([]string{b.OSIS, b.Name}, b.Aliases...) {
			byName[fold(n)] = b
		}
	}
}

// Books lists every supported book in canonical order.
func Books() []Book { return append([]Book(nil), books...) }

// BookByNumber returns the book with the given number.
func BookByNumber(n int) (Book, bool) {
	if n < 1 || n > len(books) {
		return Book{}, false
	}
	return books[n-1], true
}

// LookupBook finds a book by OSIS ID, name or common abbreviation, ignoring case, spaces and dots.
// Roman numerals and words for book numbers ("II Kings", "First John") are accepted.
func LookupBook(name string) (Book, bool) {
	b, ok := byName[fold(name)]
	if !ok {
		return Book{}, false
	}
	return *b, true
}

// fold lowercases s, drops spaces and dots, and turns leading "i/ii/iii" or "first/second/third"
// into digits.
func fold(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '.' || c == '_':
			continue
		case c >= 'A' && c <= 'Z':
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	f := string(out)
	for prefix, digit := range map[string]string{"iii": "3", "ii": "2", "i": "1", "first": "1", "second": "2", "third": "3", "fourth": "4", "iv": "4"} {
		if len(f) > len(prefix) && f[:len(prefix)] == prefix && hasSpaceAfter(s, prefix) {
			return digit + f[len(prefix):]
		}
	}
	return f
}

// hasSpaceAfter reports whether s starts with prefix (any case) followed by a space or dot, so that
// "I John" folds to "1john" while "Isaiah" stays "isaiah".
func hasSpaceAfter(s, prefix string) bool {
	if len(s) <= len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return s[len(prefix)] == ' ' || s[len(prefix)] == '.'
}
