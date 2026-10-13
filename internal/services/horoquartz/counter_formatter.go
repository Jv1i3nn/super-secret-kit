package horoquartz

import (
	"fmt"
	"strconv"
)

// CounterCategory représente une catégorie de compteurs
type CounterCategory string

const (
	CategoryLeave    CounterCategory = "leave"
	CategoryWork     CounterCategory = "work"
	CategoryTelework CounterCategory = "telework"
	CategoryOther    CounterCategory = "other"
)

var counterNames = map[string]string{
	"CNT_CP":         "Congés Payés (CP)",
	"CNT_RTT":        "RTT",
	"CNT_CET":        "Compte Épargne Temps (CET)",
	"CNT_WORKED":     "Heures travaillées",
	"CNT_CS":         "Congés sans solde",
	"CNT_RECOVERY":   "Heures de récupération",
	"CNT_ANCIENNETE": "Congés d'ancienneté",
	"CNT_TELEWORK":   "Jours de télétravail",
	"CNT_OVERTIME":   "Heures supplémentaires",
	"CNT_TRAINING":   "Heures de formation",
}

var counterCategories = map[string]CounterCategory{
	"CNT_CP":         CategoryLeave,
	"CNT_RTT":        CategoryLeave,
	"CNT_CET":        CategoryLeave,
	"CNT_CS":         CategoryLeave,
	"CNT_ANCIENNETE": CategoryLeave,
	"IJRSGCONGE":     CategoryLeave,
	"IJRSCOPRIN":     CategoryLeave,
	"IJRSACQCP":      CategoryLeave,
	"IJRSCOANCI":     CategoryLeave,
	"IJRSNBJRTT":     CategoryLeave,
	"SJRRRTT":        CategoryLeave,
	"SJRSCETCRT":     CategoryLeave,
	"SJRSCETRTR":     CategoryLeave,
	"IEQSPGCONG":     CategoryLeave,
	"IEQSPRTTJO":     CategoryLeave,
	"IEQSPGCON2":     CategoryLeave,
	"SJRRJTR":        CategoryLeave,

	"CNT_WORKED":   CategoryWork,
	"CNT_OVERTIME": CategoryWork,
	"CNT_RECOVERY": CategoryWork,
	"CNT_TRAINING": CategoryWork,
	"SJRJRIMPOT":   CategoryWork,
	"SJQRRECUPD":   CategoryWork,
	"SJRRDEPLW":    CategoryWork,

	"CNT_TELEWORK": CategoryTelework,
	"SJQSOTT":      CategoryTelework,
	"SJRDRTT":      CategoryTelework,
	"SJRPRTT":      CategoryTelework,
}

// FormatCounterName retourne le nom lisible d'un compteur
func FormatCounterName(code, label string) string {
	// Priorité au label de l'API si présent
	if label != "" && label != code {
		return label
	}

	// Sinon, utiliser le mapping
	if name, ok := counterNames[code]; ok {
		return name
	}

	// Fallback : retourner le code
	return code
}

// FormatUnit retourne l'unité formatée
func FormatUnit(unit string) string {
	switch unit {
	case "D":
		return "jours"
	case "H":
		return "heures"
	case "M":
		return "minutes"
	default:
		return unit
	}
}

// GetCounterCategory retourne la catégorie d'un compteur
func GetCounterCategory(code string) CounterCategory {
	if category, ok := counterCategories[code]; ok {
		return category
	}
	return CategoryOther
}

// FormatCounterValue retourne la valeur formatée d'un compteur
func FormatCounterValue(value, unit string) string {
	// Convertir la valeur en float
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}

	// Formater avec l'unité
	formattedUnit := FormatUnit(unit)
	return fmt.Sprintf("%.1f %s", val, formattedUnit)
}

// FormatCounter convertit un Counter en CounterDisplay
func FormatCounter(counter Counter) CounterDisplay {
	value, _ := strconv.ParseFloat(counter.Value, 64)
	return CounterDisplay{
		Name:  FormatCounterName(counter.Code, counter.Label),
		Value: value,
		Unit:  FormatUnit(counter.Unit),
		Type:  counter.Type,
	}
}

// GroupCountersByCategory regroupe les compteurs par catégorie
func GroupCountersByCategory(counters []Counter) map[CounterCategory][]Counter {
	grouped := make(map[CounterCategory][]Counter)

	for _, counter := range counters {
		category := GetCounterCategory(counter.Code)
		grouped[category] = append(grouped[category], counter)
	}

	return grouped
}

// GetCategoryTitle retourne le titre d'une catégorie
func GetCategoryTitle(category CounterCategory) string {
	switch category {
	case CategoryLeave:
		return "📅 Absences et Congés"
	case CategoryWork:
		return "⏱️  Temps de travail"
	case CategoryTelework:
		return "🏠 Télétravail"
	case CategoryOther:
		return "📊 Autres"
	default:
		return "📊 Autres"
	}
}
