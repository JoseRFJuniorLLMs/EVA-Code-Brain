package main

import (
	"context"
	"fmt"
)

// Health analytics tool implementations

var healthAnalytics *HealthAnalyticsService

func initHealthAnalytics() {
	healthAnalytics = NewHealthAnalyticsService()
}

func executeAnalyzeHealthTrend(ctx context.Context, params map[string]interface{}) (string, error) {
	idosoID, ok := params["idoso_id"].(float64)
	if !ok {
		return "", fmt.Errorf("parâmetro 'idoso_id' inválido")
	}

	metric, ok := params["metric"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'metric' inválido")
	}

	days := 30
	if d, ok := params["days"].(float64); ok {
		days = int(d)
	}

	trend, err := healthAnalytics.AnalyzeTrend(ctx, int(idosoID), metric, days)
	if err != nil {
		return "", err
	}

	// Format output
	output := fmt.Sprintf("📊 Análise de Tendência: %s (Idoso %d)\n", trend.Metric, int(idosoID))
	output += fmt.Sprintf("Período: %s\n\n", trend.Period)

	output += "**Estatísticas:**\n"
	output += fmt.Sprintf("- Pontos de dados: %d\n", trend.DataPoints)
	output += fmt.Sprintf("- Média: %.2f\n", trend.Mean)
	output += fmt.Sprintf("- Mediana: %.2f\n", trend.Median)
	output += fmt.Sprintf("- Desvio padrão: %.2f\n", trend.StdDev)
	output += fmt.Sprintf("- Mínimo: %.2f\n", trend.Min)
	output += fmt.Sprintf("- Máximo: %.2f\n\n", trend.Max)

	// Trend
	trendEmoji := "➡️"
	if trend.Trend == "increasing" {
		trendEmoji = "📈"
	} else if trend.Trend == "decreasing" {
		trendEmoji = "📉"
	}
	output += fmt.Sprintf("**Tendência:** %s %s (força: %.2f)\n\n", trendEmoji, trend.Trend, trend.TrendStrength)

	// Anomalies
	if len(trend.Anomalies) > 0 {
		output += fmt.Sprintf("⚠️  **Anomalias Detectadas:** %d\n\n", len(trend.Anomalies))
		for i, a := range trend.Anomalies {
			if i >= 5 {
				output += fmt.Sprintf("... e mais %d anomalias\n", len(trend.Anomalies)-5)
				break
			}
			severityEmoji := "🟡"
			if a.Severity == "high" {
				severityEmoji = "🔴"
			} else if a.Severity == "low" {
				severityEmoji = "🟢"
			}
			output += fmt.Sprintf("%d. %s %s - Valor: %.2f (Z-score: %.2f)\n",
				i+1, severityEmoji, a.Timestamp.Format("2006-01-02 15:04"), a.Value, a.ZScore)
		}
	} else {
		output += "✅ Nenhuma anomalia detectada\n"
	}

	return output, nil
}

func executeAssessHealthRisk(ctx context.Context, params map[string]interface{}) (string, error) {
	idosoID, ok := params["idoso_id"].(float64)
	if !ok {
		return "", fmt.Errorf("parâmetro 'idoso_id' inválido")
	}

	days := 30
	if d, ok := params["days"].(float64); ok {
		days = int(d)
	}

	assessment, err := healthAnalytics.AssessHealthRisk(ctx, int(idosoID), days)
	if err != nil {
		return "", err
	}

	// Format output
	riskEmoji := "🟢"
	if assessment.OverallRisk == "high" {
		riskEmoji = "🔴"
	} else if assessment.OverallRisk == "medium" {
		riskEmoji = "🟡"
	}

	output := fmt.Sprintf("🏥 Avaliação de Risco de Saúde (Idoso %d)\n", assessment.IdosoID)
	output += fmt.Sprintf("Período de análise: %d dias\n\n", days)
	output += fmt.Sprintf("**Risco Geral:** %s %s\n\n", riskEmoji, assessment.OverallRisk)

	// Risk factors
	if len(assessment.RiskFactors) > 0 {
		output += "**Fatores de Risco:**\n\n"
		for i, rf := range assessment.RiskFactors {
			severityEmoji := "🟡"
			if rf.Severity == "high" {
				severityEmoji = "🔴"
			} else if rf.Severity == "low" {
				severityEmoji = "🟢"
			}
			output += fmt.Sprintf("%d. %s **%s**\n", i+1, severityEmoji, rf.Factor)
			output += fmt.Sprintf("   %s\n\n", rf.Description)
		}
	} else {
		output += "✅ Nenhum fator de risco identificado\n\n"
	}

	// Recommendations
	if len(assessment.Recommendations) > 0 {
		output += "**Recomendações:**\n\n"
		for i, rec := range assessment.Recommendations {
			output += fmt.Sprintf("%d. %s\n", i+1, rec)
		}
	}

	return output, nil
}
