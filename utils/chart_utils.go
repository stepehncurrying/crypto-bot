package utils

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const Layout = "02-01-2006"
const MaxData = 250.0

func BuildJSONDataFromData(data [][]float64, crypto string) (string, error) {

	var labels []string
	var datasetValues []string
	datasetLabel := crypto

	long := len(data)
	sampling := int(math.Ceil(float64(long) / MaxData))

	for i := 0; i < long; i += sampling {
		labels = append(labels, getDisplayTime(time.Unix(int64(data[i][0]/1000), 0)))
		datasetValues = append(datasetValues, fmt.Sprintf("%.3f", data[i][1]))
	}

	var sb strings.Builder
	sb.WriteString("data:{labels:[")
	for i, v := range labels {
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("'%s'", v))
	}
	sb.WriteString(fmt.Sprintf("],datasets:[{label:'%s',data:[", datasetLabel))
	for i, v := range datasetValues {
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("'%s'", v))
	}
	sb.WriteString("]}]}")
	return sb.String(), nil
}

// Set to date format: 1 Jan 2000 00:00:00
func getDisplayTime(unixTime time.Time) string {
	splitTime := strings.Split(unixTime.Format(time.UnixDate), " ")
	if len(splitTime) == 7 { // When day is only 1 number, it fills the blank with another space
		return splitTime[3] + " " + splitTime[1] + " " + splitTime[6] + " " + splitTime[4]
	}
	return splitTime[2] + " " + splitTime[1] + " " + splitTime[5] + " " + splitTime[3]
}
