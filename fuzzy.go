package fuzzy

import(
	"fmt"
	"os"
	"bufio"
	"strings"
)

type Pair struct {
	str1 string
	str2 string
}

type Potential struct {
	term  		string
	score 		int 
	leven 		int
	method 		int 	// 0 - is word, 1 - suggest maps to input, 2 - input delete maps to dictionary, 3 - input delete maps to suggest 
}

type Model struct {
	Data 		map[string]int
	maxcount 	int
	suggest 	map[string][]string
	depth		int
	threshold 	int
	chars 		int
}

func NewModel() *Model {
	model := new(Model)
	return model.Init()
}

func (model *Model) Init() *Model {
	model.Data = make(map[string]int)
	model.suggest = make(map[string][]string)
	model.depth = 2
	model.threshold = 4 // Setting this to 1 is most accurate, but "1" is 5x more memory and 30x slower processing than "4". This is a big performance tuning knob
	return model
}

// Change the default depth value of the model. This sets how many
// character differences are indexed. The default is 2.
func (model *Model) SetDepth(val int) {
	model.depth = val
}

// Change the default threshold of the model. This is how many times
// a term must be seen before suggestions are created for it
func (model *Model) SetThreshold(val int) {
	model.threshold = val
}

func min(a, b int) int {
  if a < b {
    return a
  }
  return b
}

func max(a, b int) int {
  if a < b {
    return b
  }
  return a
}

func Levenshtein(a, b string) int {
  n, m := len(a), len(b)
  if n > m {
    a, b = b, a
    n, m = m, n
  }

  current  := make([]int, m+1)
  previous := make([]int, m+1)
  var i, j, add, delete, change int

  for i = 1; i <= m; i++ {
    copy(previous, current)
    for j = 0; j <= m; j++ { current[j] = 0 }
    current[0] = i
    for j = 1; j <= n; j++ {
      if a[j-1] == b[i-1] {
        current[j] = previous[j-1]
      } else {
        add    = previous[j] + 1
        delete = current[j-1] + 1
        change = previous[j-1] + 1
        current[j] = min(min(add, delete), change)
      }
    }
  }

  return current[n]
}

// Add an array of words to train the model in bulk
func (model *Model) Train(terms []string) {
	for _, term := range terms {
		model.TrainWord(term)
	}
}

// Train the model word by word
func (model *Model) TrainWord(term string) {
	model.Data[term]++
	// Set the max
	if model.Data[term] > model.maxcount {
		model.maxcount = model.Data[term]
	}
	// If threshold is triggered, store delete suggestion keys
	if model.Data[term] == model.threshold {
		edits := model.EditsMulti(term, model.depth)
		for _, edit := range edits {
			skip := false
			for _, hit := range model.suggest[edit] {
				if hit == term {
					// Already know about this one
					skip = true
					continue
				}
			}
			if !skip && len(edit) > 1 {
				model.suggest[edit] = append(model.suggest[edit], term)
			}
		}
	}
}

// Edits at any depth for a given term. The depth of the model is used
func (model *Model) EditsMulti(term string, depth int) []string {
	edits := Edits1(term)
	for {
		depth--
		if depth == 0 {
			break
		}
		for _, edit := range edits {
			edits2 := Edits1(edit)
			for _, edit2 := range edits2 {
				edits = append(edits, edit2)
			}
		}
	}
	return edits
}

// Edits1 creates a set of terms that are 1 char delete from the input term
func Edits1(word string) []string {

  splits := []Pair{}
  for i := 0; i <= len(word); i++ {
    splits = append(splits, Pair{word[:i], word[i:]})
  }

  total_set := []string{}
  for _, elem := range splits {

	//deletion
	if len(elem.str2) > 0 {
		total_set = append(total_set, elem.str1+elem.str2[1:])
	} else {
		total_set = append(total_set, elem.str1)
	}

  }
  return total_set
}

func (model *Model) score(input string) int {
	if score, ok := model.Data[input]; ok {
		return score
	}
	return 0
}

// From a group of potentials, work out the most likely result
func best(input string, potential map[string]*Potential) string {
	best := ""
	bestcalc := 0
	for i := 0; i < 4; i++ {
		for _, pot := range potential {
			if pot.leven == 0 {
				return pot.term
			} else if pot.leven == i {
				if pot.score > bestcalc {
					bestcalc = pot.score
					// If the first letter is the same, that's a good sign. Bias these potentials
					
					if pot.term[0] == input[0] {
						bestcalc += bestcalc * 100
					}
					
					best = pot.term
				}
			}
		}
		if bestcalc > 0 {
			return best
		}
	}

	return best
}

// Test an input, if we get it wrong, look at why it is wrong. This 
// function returns a bool indicating if the guess was correct as well 
// as the term it is suggesting
func (model *Model) CheckKnown(input string, correct string) bool {
	suggestions := model.suggestPotential(input, true)
	best := best(input, suggestions)
	if best == correct {
		// This guess is correct
		fmt.Printf("Input correctly maps to correct term")
		return true
	}
	if pot, ok := suggestions[correct]; !ok {
		if model.score(correct) > 0 {
			fmt.Printf("\"%v\" - %v (%v) not in the suggestions. (%v) best option.\n", input, correct, model.score(correct), best)
			for _, sugg := range suggestions {
				fmt.Printf("	%v\n", sugg)
			}
		} else {
			fmt.Printf("\"%v\" - Not in dictionary\n", correct)
		}
	} else {
		fmt.Printf("\"%v\" - (%v) suggested, should however be (%v).\n", input, suggestions[best], pot)
	}
	return false
}


// For a given input term, suggest some alternatives. If exhaustive, each of the 4
// cascading checks will be performed and all potentials will be sorted accordingly
func (model *Model) suggestPotential(input string, exhaustive bool) map[string]*Potential {
	input = strings.ToLower(input)
	suggestions := make(map[string]*Potential, 20)

	// 0 - If this is a dictionary term we're all good, no need to go further
	if model.score(input) > 5 {
		suggestions[input] = &Potential{term : input, score : model.score(input), leven : 0, method : 0}
		if !exhaustive {
			return suggestions
		}
	}

	// 1 - See if the input matches a "suggest" key
	if sugg, ok := model.suggest[input]; ok {
		for _, pot := range sugg {
			if _, ok := suggestions[pot]; !ok {
				suggestions[pot] = &Potential{term : pot, score : model.score(pot), leven : Levenshtein(input, pot), method : 1}
			}
		}

		if !exhaustive {
			return suggestions
		}
	}

	// 2 - See if edit1 matches input
	max := 0
	edits := model.EditsMulti(input, model.depth)
	for _, edit := range edits {
		score := model.score(edit)
		if score > 0 && len(edit) > 2 { 
			if _, ok := suggestions[edit]; !ok {
				suggestions[edit] = &Potential{term : edit, score : score, leven : Levenshtein(input, edit), method : 2}
			}
			if (score > max) {
				max = score
			}
		}
	}
	if max > 0 {
		if !exhaustive {
			return suggestions
		}
	}

	// 3 - No hits on edit1 distance, look for transposes and replaces
	// Note: these are more complex, we need to check the guesses
	// more thoroughly, e.g. levals=[valves] in a raw sense, which
	// is incorrect
	for _, edit := range edits {
		if sugg, ok := model.suggest[edit]; ok {
			// Is this a real transpose or replace?
			for _, pot := range sugg {
				lev := Levenshtein(input, pot)
				if lev <= model.depth + 1 { // The +1 doesn't seem to impact speed, but has greater coverage when the depth is not sufficient to make suggestions
					if _, ok := suggestions[pot]; !ok {
						suggestions[pot] = &Potential{term : pot, score : model.score(pot), leven : lev, method : 3}
					}
				}
			}
		}
	}
	return suggestions
}

func (model *Model) Suggestions(input string, exhaustive bool) []string {
	suggestions := model.suggestPotential(input, exhaustive)
	output := make([]string, 10)
	for _, suggestion := range suggestions {
		output = append(output, suggestion.term)
	}
	return output
}

// Return the most likely correction for the input term
func (model *Model) SpellCheck(input string) string {
	suggestions := model.suggestPotential(input, false)
	return best(input, suggestions)
}

func SampleEnglish() []string {
	var out []string 
	file, err := os.Open("data/big.txt") 
    if (err != nil) { 
    	fmt.Println(err)
        return out
    }
    reader := bufio.NewReader(file)
    scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanWords)
	// Count the words.
	count := 0
	for scanner.Scan() {
		word := strings.Trim(scanner.Text(), "=+'|_,-!;:\"?.")
		out = append(out, strings.ToLower(word))
		count++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}

	return out
}





