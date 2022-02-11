package updator

type Ratio string

const (
	RatioLow    Ratio = "low"
	RatioMedium Ratio = "medium"
	RatioHigh   Ratio = "high"
)

type Quality string

const (
	QualityPoor    Quality = "poor"
	QualityGeneral Quality = "general"
	QualityGood    Quality = "good"
)

type Condition struct {
	ratio   Ratio
	quality Quality
}

type Delta float32

type Rules map[Condition]Delta

var initialRuleSets = Rules{
	Condition{RatioLow, QualityPoor}:       0.2,
	Condition{RatioLow, QualityGeneral}:    0,
	Condition{RatioLow, QualityGood}:       -0.2,
	Condition{RatioMedium, QualityPoor}:    0.5,
	Condition{RatioMedium, QualityGeneral}: 0.2,
	Condition{RatioMedium, QualityGood}:    0,
	Condition{RatioHigh, QualityPoor}:      1,
	Condition{RatioHigh, QualityGeneral}:   0.5,
	Condition{RatioHigh, QualityGood}:      0.2,
}

func getRules() *Rules {
	return &initialRuleSets
}

type Weight float64

func min(w1, w2 Weight) Weight {
	if w1 < w2 {
		return w1
	} else {
		return w2
	}
}

// y = f(x) = 1.0,  0 < x <= 0.6
//			= -2.5 * x + 2.5, 0.6 <= x < 1
//          = 0, others
func lowRatioFunc(ratio float64) Weight {
	if 0 < ratio && ratio <= 0.6 {
		return 1.0
	} else if 0.6 <= ratio && ratio < 1 {
		return Weight(-2.5*ratio + 2.5)
	} else {
		return 0.0
	}
}

// y = f(x) = 2.5 * x - 1.5, 0.6 < x <= 1
//			= -2.5 * x + 3.5, 1 < x <= 1.4
//			= 0, others
func mediumRatioFunc(ratio float64) Weight {
	if 0.6 < ratio && ratio <= 1 {
		return Weight(2.5*ratio - 1.5)
	} else if 1 < ratio && ratio <= 1.4 {
		return Weight(-2.5*ratio + 3.5)
	} else {
		return 0
	}
}

// y = f(x) = 2.5 * x - 2.5, 1 < x <= 1.4
//			= 1, x > 1.4
//			= 0, others
func highRatioFunc(ratio float64) Weight {
	if 1 < ratio && ratio <= 1.4 {
		return Weight(2.5*ratio - 2.5)
	} else if ratio > 1.4 {
		return 1
	} else {
		return 0
	}
}

// y = f(x) = 1, x > 1
//			= 5 * x - 4, 0.8 < x <= 1
//			= 0, others
func poorQualityFunc(quality float64) Weight {
	if quality > 1 {
		return 1
	} else if 0.8 < quality && quality <= 1 {
		return Weight(5*quality - 4)
	} else {
		return 0
	}
}

// y = f(x) = 5 * x - 3, 0.6 < x <= 0.8
//			= -5 * x + 5, 0.8 < x <= 1
//			= 0, others
func generalQualityFunc(quality float64) Weight {
	if 0.6 < quality && quality <= 0.8 {
		return Weight(5*quality - 3)
	} else if 0.8 < quality && quality <= 1 {
		return Weight(-5*quality + 5)
	} else {
		return 0
	}
}

// y = f(x) = 1, x <= 0.6
//			= -5 * x + 4, 0.6 < x <= 0.8
//			= 0, others
func goodQualityFunc(quality float64) Weight {
	if quality <= 0.6 {
		return 1
	} else if 0.6 < quality && quality <= 0.8 {
		return Weight(-5*quality + 4)
	} else {
		return 0
	}
}

func getRatioMap(ratio float64) map[Ratio]Weight {
	var m = make(map[Ratio]Weight)
	m[RatioHigh] = highRatioFunc(ratio)
	m[RatioMedium] = mediumRatioFunc(ratio)
	m[RatioLow] = lowRatioFunc(ratio)
	return m
}

func getQualityMap(quality float64) map[Quality]Weight {
	var m = make(map[Quality]Weight)
	m[QualityGood] = goodQualityFunc(quality)
	m[QualityGeneral] = generalQualityFunc(quality)
	m[QualityPoor] = poorQualityFunc(quality)
	return m
}

type Result struct {
	delta  Delta
	weight Weight
}

func CalculateDelta(ratio, quality float64) Delta {
	ratioMap := getRatioMap(ratio)
	qualityMap := getQualityMap(quality)

	resultMap := make(map[Condition]Result)
	rules := getRules()
	for cond, delta := range *rules {
		weight := min(ratioMap[cond.ratio], qualityMap[cond.quality])
		resultMap[cond] = Result{delta: delta, weight: weight}
	}

	sum := 0.0
	w := 0.0
	for _, r := range resultMap {
		sum += float64(r.delta) * float64(r.weight)
		w += float64(r.weight)
	}

	return Delta(sum / w)
}
