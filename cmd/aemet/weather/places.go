// Package weather resolves Spanish municipalities and turns AEMET forecasts
// into compact structures and text shared by the CLI commands and the MCP server.
package weather

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/rubiojr/aemet-go"
)

// Place is a Spanish municipality known to AEMET.
type Place struct {
	ID         string `json:"id" jsonschema:"5-digit INE municipality code"`
	Name       string `json:"name"`
	Province   string `json:"province"`
	Population int    `json:"population,omitempty"`
}

func (p Place) String() string {
	return fmt.Sprintf("%s (%s, id %s)", p.Name, p.Province, p.ID)
}

// AmbiguousError reports a query matching several municipalities.
type AmbiguousError struct {
	Query      string
	Candidates []Place
}

func (e *AmbiguousError) Error() string {
	names := make([]string, len(e.Candidates))
	for i, c := range e.Candidates {
		names[i] = c.String()
	}
	return fmt.Sprintf("%q matches several municipalities: %s; retry with an id or a province",
		e.Query, strings.Join(names, "; "))
}

// MaxQueryLen bounds municipality and province queries.
const MaxQueryLen = 100

const maxCandidates = 10

type entry struct {
	place    Place
	variants []string // folded names the place is searchable by
	province string   // folded province names
}

var idRe = regexp.MustCompile(`^[0-9]{5}$`)

// provinces maps INE province codes, the first two digits of a municipality
// code, to province names. Alternate names follow a slash.
var provinces = map[string]string{
	"01": "Araba/Álava", "02": "Albacete", "03": "Alicante/Alacant", "04": "Almería",
	"05": "Ávila", "06": "Badajoz", "07": "Illes Balears/Islas Baleares", "08": "Barcelona",
	"09": "Burgos", "10": "Cáceres", "11": "Cádiz", "12": "Castellón/Castelló",
	"13": "Ciudad Real", "14": "Córdoba", "15": "A Coruña/La Coruña", "16": "Cuenca",
	"17": "Girona/Gerona", "18": "Granada", "19": "Guadalajara", "20": "Gipuzkoa/Guipúzcoa",
	"21": "Huelva", "22": "Huesca", "23": "Jaén", "24": "León",
	"25": "Lleida/Lérida", "26": "La Rioja", "27": "Lugo", "28": "Madrid",
	"29": "Málaga", "30": "Murcia", "31": "Navarra/Nafarroa", "32": "Ourense/Orense",
	"33": "Asturias", "34": "Palencia", "35": "Las Palmas", "36": "Pontevedra",
	"37": "Salamanca", "38": "Santa Cruz de Tenerife", "39": "Cantabria", "40": "Segovia",
	"41": "Sevilla", "42": "Soria", "43": "Tarragona", "44": "Teruel",
	"45": "Toledo", "46": "Valencia/València", "47": "Valladolid", "48": "Bizkaia/Vizcaya",
	"49": "Zamora", "50": "Zaragoza", "51": "Ceuta", "52": "Melilla",
}

// articles that AEMET moves to the end of a name: "Coruña, A".
var articles = map[string]bool{
	"a": true, "o": true, "as": true, "os": true, "el": true, "la": true, "los": true,
	"las": true, "l'": true, "les": true, "els": true, "es": true, "sa": true, "ses": true,
}

var folder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "’", "'", "·", ".",
)

// fold lowercases s, strips accents and collapses whitespace.
func fold(s string) string {
	return strings.Join(strings.Fields(folder.Replace(strings.ToLower(s))), " ")
}

// nameVariants returns the folded names a municipality is searchable by:
// the full name, each slash-separated alternate, and alternates with a
// trailing article moved to the front ("Coruña, A" → "a coruña").
func nameVariants(name string) []string {
	variants := []string{fold(name)}
	for part := range strings.SplitSeq(name, "/") {
		part = fold(part)
		variants = append(variants, part)
		if i := strings.LastIndex(part, ", "); i > 0 {
			base, art := part[:i], part[i+2:]
			if articles[art] {
				sep := " "
				if strings.HasSuffix(art, "'") {
					sep = ""
				}
				variants = append(variants, art+sep+base)
			}
		}
	}
	slices.Sort(variants)
	return slices.Compact(variants)
}

var loadIndex = sync.OnceValues(func() ([]entry, error) {
	all, err := aemet.GetAllMunicipalities()
	if err != nil {
		return nil, err
	}
	idx := make([]entry, 0, len(all))
	for _, m := range all {
		id := strings.TrimPrefix(m.ID, "id")
		pop, _ := strconv.Atoi(m.NumHab)
		prov := provinces[id[:min(2, len(id))]]
		idx = append(idx, entry{
			place:    Place{ID: id, Name: m.Name, Province: prov, Population: pop},
			variants: nameVariants(m.Name),
			province: fold(prov),
		})
	}
	return idx, nil
})

// IsID reports whether s looks like a 5-digit INE municipality code.
func IsID(s string) bool {
	return idRe.MatchString(strings.TrimSpace(s))
}

// Lookup returns the municipality with the given INE code.
func Lookup(id string) (Place, error) {
	id = strings.TrimSpace(id)
	if !IsID(id) {
		return Place{}, fmt.Errorf("invalid municipality id %q: want a 5-digit INE code such as 28079", id)
	}
	idx, err := loadIndex()
	if err != nil {
		return Place{}, err
	}
	for _, e := range idx {
		if e.place.ID == id {
			return e.place, nil
		}
	}
	return Place{}, fmt.Errorf("unknown municipality id %s", id)
}

const noMatch = 4

// rank scores how well the folded query q matches an entry; lower is better.
func (e entry) rank(q string) int {
	best := noMatch
	for _, v := range e.variants {
		switch {
		case v == q:
			return 0
		case strings.HasPrefix(v, q):
			best = min(best, 1)
		case strings.Contains(v, " "+q):
			best = min(best, 2)
		case strings.Contains(v, q):
			best = min(best, 3)
		}
	}
	return best
}

type match struct {
	place Place
	rank  int
}

func checkQuery(query, province string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("municipality name is required")
	}
	if len(query) > MaxQueryLen || len(province) > MaxQueryLen {
		return fmt.Errorf("query too long (max %d bytes)", MaxQueryLen)
	}
	return nil
}

func search(query, province string) ([]match, error) {
	if err := checkQuery(query, province); err != nil {
		return nil, err
	}
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	q, prov := fold(query), fold(province)
	var res []match
	for _, e := range idx {
		if prov != "" && !strings.Contains(e.province, prov) {
			continue
		}
		if r := e.rank(q); r < noMatch {
			res = append(res, match{e.place, r})
		}
	}
	slices.SortStableFunc(res, func(a, b match) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if a.place.Population != b.place.Population {
			return b.place.Population - a.place.Population
		}
		return strings.Compare(a.place.Name, b.place.Name)
	})
	return res, nil
}

// Search returns up to limit municipalities matching query, best first:
// exact names, then prefixes, word prefixes and substrings, larger towns
// first within each group. Matching ignores case and accents. A non-empty
// province restricts results to provinces whose name contains it.
func Search(query, province string, limit int) ([]Place, error) {
	res, err := search(query, province)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	places := make([]Place, len(res))
	for i, m := range res {
		places[i] = m.place
	}
	return places, nil
}

// Resolve returns the single municipality a name or INE code refers to.
// A unique exact name wins over partial matches. It returns an
// *AmbiguousError listing candidates when the query is not specific enough.
func Resolve(query, province string) (Place, error) {
	if IsID(query) {
		return Lookup(query)
	}
	res, err := search(query, province)
	if err != nil {
		return Place{}, err
	}
	switch {
	case len(res) == 0:
		return Place{}, notFound(query, province)
	case len(res) == 1 || res[0].rank == 0 && res[1].rank > 0:
		return res[0].place, nil
	}
	n := len(res)
	if res[0].rank == 0 {
		// Several exact matches: offer only those.
		if i := slices.IndexFunc(res, func(m match) bool { return m.rank > 0 }); i > 0 {
			n = i
		}
	}
	n = min(n, maxCandidates)
	cands := make([]Place, n)
	for i := range cands {
		cands[i] = res[i].place
	}
	return Place{}, &AmbiguousError{Query: query, Candidates: cands}
}

func notFound(query, province string) error {
	if province != "" {
		return fmt.Errorf("no municipality matches %q in province %q", query, province)
	}
	return fmt.Errorf("no municipality matches %q", query)
}
