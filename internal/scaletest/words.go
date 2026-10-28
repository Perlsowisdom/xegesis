package scaletest

import (
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// headWords are frequent English words, roughly in frequency order, followed by words common in
// theology. The long tail of the vocabulary is made of invented words, so the text has the shape of
// real prose (a few words very common, most words rare) without copying any real book.
const headWords = `the of and to a in that is was he for it with as his on be at by i this had not are but
from or have an they which one you were her all she there would their we him been has when who will
more no if out so said what up its about into than them can only other new some could time these two
may then do first any my now such like our over man me even most made after also did many before must
through back years where much your way well down should because each just those people how too little
state good very make world still own see men work long get here between both life being under never
day same another know while last might us great old year off come since against go came right used
take three himself few house use during without again place around however home small found thought
went say part once general high upon school every does got united left number course war until always
away something fact though water less public put think almost hand enough far took head yet government
system better set told nothing night end why called didn't eyes find going look asked later knew point
next program city business give group toward young days let room president side social given present
several order national possible rather second face per among form important often things looking early
white case john become large big need four within felt along children saw best church ever least
power development light seemed family interest want members mind country area others although turn
done open god service certain kind problem began different door thus help means sense whole matter
perhaps itself york times law human line above name example action company hands local show whether
five history gave today either act feet across taken past quite anything seen having death experience
body word half really week field car words already themselves information tell together college shall
money period held keep sure probably free seems real behind cannot political air question making office
brought whose special heard major problems ago became federal moment study available known result
street economic boy position reason change south board individual job areas society west close turned
love community true court force full seem am wife age future voice center woman control common policy
necessary following front sometimes six girl clear further land run students provide feel party able
mother music education university child effect level stood military town short morning total outside
rate figure class art century north washington leave therefore plan top evidence million sound black
strong hard various says believe type value play surface soon mean near lines table peace modern tax
road red book personal process situation minister idea increase english alone women gone nor amount
doing porch faith grace lord christ jesus spirit holy scripture gospel sin salvation prayer heaven
earth glory mercy righteous righteousness covenant law prophet apostle israel king kingdom temple
sacrifice blood cross resurrection hope peace truth commandment disciple church believer baptism
judgment wisdom creation fall redemption atonement justification sanctification eternal divine heart
soul flesh worship praise father son spirit throne angel servant shepherd sheep lamb bread wine
vine branch light darkness life death grave mountain wilderness river city jerusalem babylon egypt
moses abraham david paul peter isaiah jeremiah psalm proverb parable miracle sermon epistle letter
chapter verse text reading translation hebrew greek commentary doctrine theology tradition council
reformation preacher pastor elder deacon congregation mission nations gentiles jews pharisees`

var syllables = strings.Fields(`ba be bi bo bu da de di do du ka ke ki ko ku la le li lo lu ma me mi mo mu
na ne ni no nu pa pe pi po pu ra re ri ro ru sa se si so su ta te ti to tu va ve vi vo vu tha the thi
tho ar er ir or ur an en in on un al el il ol ul as es is os us ath eth ith oth`)

// vocab is a Zipf-distributed vocabulary.
type vocab struct {
	words []string
	cum   []float64 // cumulative probability by rank
}

func newVocab(size int, seed uint64) *vocab {
	seen := map[string]bool{}
	var words []string
	for _, w := range strings.Fields(headWords) {
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	r := rand.New(rand.NewPCG(seed, 7))
	for len(words) < size {
		n := 2 + r.IntN(3)
		var b strings.Builder
		for range n {
			b.WriteString(syllables[r.IntN(len(syllables))])
		}
		if w := b.String(); !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	v := &vocab{words: words, cum: make([]float64, len(words))}
	total := 0.0
	for i := range words {
		total += 1 / math.Pow(float64(i+1), 1.05)
		v.cum[i] = total
	}
	for i := range v.cum {
		v.cum[i] /= total
	}
	return v
}

func (v *vocab) word(r *rand.Rand) string {
	return v.words[sort.SearchFloat64s(v.cum, r.Float64())%len(v.words)]
}
