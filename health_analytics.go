package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// HealthAnalyticsService handles health data analysis
type HealthAnalyticsService struct{}

// HealthDataPoint represents a single health measurement
type HealthDataPoint struct {
	Timestamp time.Time
	Value     float64
	Type      string
}

// TrendAnalysis represents statistical analysis of health data
type TrendAnalysis struct {
	Metric        string
	Period        string
	DataPoints    int
	Mean          float64
	Median        float64
	StdDev        float64
	Min           float64
	Max           float64
	Trend         string // "increasing", "decreasing", "stable"
	TrendStrength float64
	Anomalies     []Anomaly
}

// Anomaly represents an unusual data point
type Anomaly struct {
	Timestamp time.Time
	Value     float64
	ZScore    float64
	Severity  string // "low", "medium", "high"
}

// RiskAssessment represents health risk evaluation
type RiskAssessment struct {
	IdosoID         int
	OverallRisk     string // "low", "medium", "high"
	RiskFactors     []RiskFactor
	Recommendations []string
}

// RiskFactor represents a specific health risk
type RiskFactor struct {
	Factor      string
	Severity    string
	Description string
}

// NewHealthAnalyticsService creates a new analytics service
func NewHealthAnalyticsService() *HealthAnalyticsService {
	return &HealthAnalyticsService{}
}

// AnalyzeTrend analyzes health metric trends over time
func (has *HealthAnalyticsService) AnalyzeTrend(ctx context.Context, idosoID int, metric string, days int) (*TrendAnalysis, error) {
	// Fetch data from database
	dataPoints, err := has.fetchHealthData(ctx, idosoID, metric, days)
	if err != nil {
		return nil, err
	}

	if len(dataPoints) == 0 {
		return nil, fmt.Errorf("nenhum dado encontrado para %s nos últimos %d dias", metric, days)
	}

	// Calculate statistics
	values := make([]float64, len(dataPoints))
	for i, dp := range dataPoints {
		values[i] = dp.Value
	}

	mean := calculateMean(values)
	median := calculateMedian(values)
	stdDev := calculateStdDev(values, mean)
	min, max := calculateMinMax(values)

	// Detect trend
	trend, strength := detectTrend(dataPoints)

	// Detect anomalies
	anomalies := detectAnomalies(dataPoints, mean, stdDev)

	return &TrendAnalysis{
		Metric:        metric,
		Period:        fmt.Sprintf("%d dias", days),
		DataPoints:    len(dataPoints),
		Mean:          mean,
		Median:        median,
		StdDev:        stdDev,
		Min:           min,
		Max:           max,
		Trend:         trend,
		TrendStrength: strength,
		Anomalies:     anomalies,
	}, nil
}

// fetchHealthData retrieves health data from database
func (has *HealthAnalyticsService) fetchHealthData(ctx context.Context, idosoID int, metric string, days int) ([]HealthDataPoint, error) {
	var query string
	var args []interface{}

	// Map metric to table and column
	switch metric {
	case "bpm", "batimentos", "heart_rate":
		query = `
			SELECT data_hora, CAST(valor AS FLOAT) as valor
			FROM sinais_vitais_health
			WHERE idoso_id = $1 
			  AND tipo = 'bpm'
			  AND data_hora >= NOW() - INTERVAL '%d days'
			ORDER BY data_hora ASC`
		query = fmt.Sprintf(query, days)
		args = []interface{}{idosoID}

	case "spo2", "oxigenio":
		query = `
			SELECT data_hora, CAST(valor AS FLOAT) as valor
			FROM sinais_vitais_health
			WHERE idoso_id = $1 
			  AND tipo = 'spo2'
			  AND data_hora >= NOW() - INTERVAL '%d days'
			ORDER BY data_hora ASC`
		query = fmt.Sprintf(query, days)
		args = []interface{}{idosoID}

	case "passos", "steps":
		query = `
			SELECT data_hora, CAST(valor AS FLOAT) as valor
			FROM sinais_vitais_health
			WHERE idoso_id = $1 
			  AND tipo = 'passos'
			  AND data_hora >= NOW() - INTERVAL '%d days'
			ORDER BY data_hora ASC`
		query = fmt.Sprintf(query, days)
		args = []interface{}{idosoID}

	case "sono", "sleep":
		query = `
			SELECT data_hora, CAST(valor AS FLOAT) as valor
			FROM sinais_vitais_health
			WHERE idoso_id = $1 
			  AND tipo = 'sono'
			  AND data_hora >= NOW() - INTERVAL '%d days'
			ORDER BY data_hora ASC`
		query = fmt.Sprintf(query, days)
		args = []interface{}{idosoID}

	default:
		return nil, fmt.Errorf("métrica desconhecida: %s", metric)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar dados: %w", err)
	}
	defer rows.Close()

	var dataPoints []HealthDataPoint
	for rows.Next() {
		var dp HealthDataPoint
		var timestamp time.Time
		var value float64

		if err := rows.Scan(&timestamp, &value); err != nil {
			continue
		}

		dp.Timestamp = timestamp
		dp.Value = value
		dp.Type = metric
		dataPoints = append(dataPoints, dp)
	}

	return dataPoints, nil
}

// AssessHealthRisk evaluates overall health risk
func (has *HealthAnalyticsService) AssessHealthRisk(ctx context.Context, idosoID int, days int) (*RiskAssessment, error) {
	assessment := &RiskAssessment{
		IdosoID:         idosoID,
		RiskFactors:     []RiskFactor{},
		Recommendations: []string{},
	}

	// Analyze heart rate
	bpmTrend, err := has.AnalyzeTrend(ctx, idosoID, "bpm", days)
	if err == nil {
		if bpmTrend.Mean > 100 {
			assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
				Factor:      "Frequência Cardíaca Elevada",
				Severity:    "medium",
				Description: fmt.Sprintf("Média de %.0f BPM (normal: 60-100)", bpmTrend.Mean),
			})
			assessment.Recommendations = append(assessment.Recommendations,
				"Consultar cardiologista para avaliação")
		}

		if bpmTrend.Trend == "increasing" && bpmTrend.TrendStrength > 0.5 {
			assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
				Factor:      "Tendência de Aumento nos Batimentos",
				Severity:    "low",
				Description: "Batimentos cardíacos em tendência crescente",
			})
		}

		if len(bpmTrend.Anomalies) > 0 {
			highSeverityCount := 0
			for _, a := range bpmTrend.Anomalies {
				if a.Severity == "high" {
					highSeverityCount++
				}
			}
			if highSeverityCount > 0 {
				assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
					Factor:      "Anomalias Cardíacas Detectadas",
					Severity:    "high",
					Description: fmt.Sprintf("%d episódios anormais detectados", highSeverityCount),
				})
				assessment.Recommendations = append(assessment.Recommendations,
					"Atenção urgente: anomalias cardíacas detectadas")
			}
		}
	}

	// Analyze SpO2
	spo2Trend, err := has.AnalyzeTrend(ctx, idosoID, "spo2", days)
	if err == nil {
		if spo2Trend.Mean < 95 {
			assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
				Factor:      "Saturação de Oxigênio Baixa",
				Severity:    "high",
				Description: fmt.Sprintf("Média de %.1f%% (normal: >95%%)", spo2Trend.Mean),
			})
			assessment.Recommendations = append(assessment.Recommendations,
				"Avaliação respiratória urgente necessária")
		}
	}

	// Analyze activity (steps)
	stepsTrend, err := has.AnalyzeTrend(ctx, idosoID, "passos", days)
	if err == nil {
		if stepsTrend.Mean < 2000 {
			assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
				Factor:      "Baixa Atividade Física",
				Severity:    "low",
				Description: fmt.Sprintf("Média de %.0f passos/dia (recomendado: >5000)", stepsTrend.Mean),
			})
			assessment.Recommendations = append(assessment.Recommendations,
				"Incentivar caminhadas leves diárias")
		}

		if stepsTrend.Trend == "decreasing" && stepsTrend.TrendStrength > 0.5 {
			assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
				Factor:      "Redução de Atividade",
				Severity:    "medium",
				Description: "Atividade física em declínio",
			})
			assessment.Recommendations = append(assessment.Recommendations,
				"Investigar possíveis causas de redução de mobilidade")
		}
	}

	// Analyze sleep
	sleepTrend, err := has.AnalyzeTrend(ctx, idosoID, "sono", days)
	if err == nil {
		if sleepTrend.Mean < 6 {
			assessment.RiskFactors = append(assessment.RiskFactors, RiskFactor{
				Factor:      "Sono Insuficiente",
				Severity:    "medium",
				Description: fmt.Sprintf("Média de %.1f horas/noite (recomendado: 7-9h)", sleepTrend.Mean),
			})
			assessment.Recommendations = append(assessment.Recommendations,
				"Avaliar qualidade do sono e possíveis distúrbios")
		}
	}

	// Calculate overall risk
	highRiskCount := 0
	mediumRiskCount := 0
	for _, rf := range assessment.RiskFactors {
		if rf.Severity == "high" {
			highRiskCount++
		} else if rf.Severity == "medium" {
			mediumRiskCount++
		}
	}

	if highRiskCount > 0 {
		assessment.OverallRisk = "high"
	} else if mediumRiskCount > 1 {
		assessment.OverallRisk = "medium"
	} else if len(assessment.RiskFactors) > 0 {
		assessment.OverallRisk = "low"
	} else {
		assessment.OverallRisk = "low"
		assessment.Recommendations = append(assessment.Recommendations,
			"Manter acompanhamento regular")
	}

	return assessment, nil
}

// Statistical helper functions

func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}

func calculateMinMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func detectTrend(dataPoints []HealthDataPoint) (string, float64) {
	if len(dataPoints) < 2 {
		return "stable", 0
	}

	// Simple linear regression
	n := float64(len(dataPoints))
	var sumX, sumY, sumXY, sumX2 float64

	for i, dp := range dataPoints {
		x := float64(i)
		y := dp.Value
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate slope
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	// Normalize slope by mean value
	mean := sumY / n
	normalizedSlope := slope / mean

	// Determine trend
	trend := "stable"
	strength := math.Abs(normalizedSlope)

	if normalizedSlope > 0.01 {
		trend = "increasing"
	} else if normalizedSlope < -0.01 {
		trend = "decreasing"
	}

	return trend, strength
}

func detectAnomalies(dataPoints []HealthDataPoint, mean, stdDev float64) []Anomaly {
	var anomalies []Anomaly

	for _, dp := range dataPoints {
		zScore := (dp.Value - mean) / stdDev

		// Z-score thresholds
		if math.Abs(zScore) > 2 {
			severity := "low"
			if math.Abs(zScore) > 3 {
				severity = "high"
			} else if math.Abs(zScore) > 2.5 {
				severity = "medium"
			}

			anomalies = append(anomalies, Anomaly{
				Timestamp: dp.Timestamp,
				Value:     dp.Value,
				ZScore:    zScore,
				Severity:  severity,
			})
		}
	}

	return anomalies
}
