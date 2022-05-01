package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"golang.org/x/exp/maps"

	"github.com/joeshaw/iso8601"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func recordMetrics() {
	go func() {
		for {
			opsProcessed.Inc()
			time.Sleep(2 * time.Second)
		}
	}()
	getAllSensorStates()
}

var (
	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "myapp_processed_ops_total",
		Help: "The total number of processed events",
	})
	apiKey        string
	conbeeURI     *string
	promNamespace = "conbee"
)

type conbeeSensor struct {
	Name             string `json:"name"`
	Etag             string `json:"etag"`
	Manufacturername string `json:"manufacturername"`
	Modelid          string `json:"modelid"`
	Swversion        string `json:"swversion"`
	Type             string `json:"type"`
	Uniqueid         string `json:"uniqueid"`
	State            struct {
		Airquality         string       `json:"airquality,omitempty"`
		Airqualityppb      uint16       `json:"airqualityppb,omitempty"`
		Alarm              bool         `json:"alarm,omitempty"`
		Angle              int          `json:"angle,omitempty"`
		Buttonevent        int          `json:"buttonevent,omitempty"`
		Carbonmonoxide     bool         `json:"carbonmonoxide,omitempty"`
		Consumption        int          `json:"consumption,omitempty"`
		Current            int          `json:"current,omitempty"`
		Dark               bool         `json:"dark,omitempty"`
		Daylight           bool         `json:"daylight,omitempty"`
		Errorcode          string       `json:"errorcode,omitempty"`
		Eventduration      int          `json:"eventduration,omitempty"`
		Fanmode            string       `json:"fanmode,omitempty"`
		Fire               bool         `json:"fire,omitempty"`
		Floortemperature   int          `json:"floortemperature,omitempty"`
		Gesture            int          `json:"gesture,omitempty"`
		Heating            bool         `json:"heating,omitempty"`
		Humidity           int          `json:"humidity,omitempty"`
		Lastset            iso8601.Time `json:"lastset,omitempty"`
		Lastupdated        iso8601.Time `json:"lastupdated,omitempty"`
		Lightlevel         int          `json:"lightlevel,omitempty"`
		Localtime          iso8601.Time `json:"localtime,omitempty"`
		Lowbattery         bool         `Btoin:"lowbattery,omitempty"`
		Lux                int          `json:"lux,omitempty"`
		Mountingmodeactive bool         `json:"mountingmodeactive,omitempty"`
		On                 bool         `json:"on,omitempty"`
		Open               bool         `json:"open,omitempty"`
		Orientation        []int        `json:"orientation,omitempty"`
		Power              int          `json:"power,omitempty"`
		Presence           bool         `json:"presence,omitempty"`
		Pressure           int          `json:"pressure,omitempty"`
		Tampered           bool         `Btoi:"tampered,omitempty"`
		Temperature        int          `json:"temperature,omitempty"`
		Tiltangle          int          `json:"tiltangle,omitempty"`
		Utc                iso8601.Time `json:"utc,omitempty"`
		Valve              int          `json:"valve,omitempty"`
		Vibration          bool         `json:"vibration,omitempty"`
		Vibrationstrength  int          `json:"vibrationstrength,omitempty"`
		Voltage            int          `json:"voltage,omitempty"`
		Water              bool         `json:"water,omitempty"`
		Windowopen         string       `json:"windowopen,omitempty"`
		X                  int          `json:"x,omitempty"`
		Y                  int          `json:"y,omitempty"`
	} `json:"state"`
	Config struct {
		Battery       int  `json:"battery"`
		Configured    bool `json:"configured"`
		Enrolled      int  `json:"enrolled"`
		Offset        int  `json:"offset"`
		On            bool `json:"on"`
		Reachable     bool `json:"reachable"`
		SunriseOffset int  `json:"sunriseoffset"`
		SunsetOffset  int  `json:"sunsetoffset"`
		Temperature   int  `json:"temperature"`
	} `json:"config"`
}

func main() {
	conbeeURI = flag.String("conbee-uri", "", "Conbee URI - if unset, will try discovery")
	flag.Parse()
	apiKey = os.Getenv("CONBEE_API_KEY")
	if apiKey == "" {
		log.Fatal("The required environment variable 'CONBEE_API_KEY' is not set!")
	}

	if *conbeeURI == "" {
		resp, err := http.Get("https://phoscon.de/discover")
		if err != nil {
			log.WithError(err).Fatal("Issue when discovering conbee sticks")
		}
		if resp.Body != nil {
			defer resp.Body.Close()
		}
		body, readErr := ioutil.ReadAll(resp.Body)
		if readErr != nil {
			log.WithError(err).Fatal("Could not read discover body")
		}
		type discoverResult struct {
			Internalipaddress string `json:"internalipaddress"`
			Internalport      int    `json:"internalport"`
			Name              string `json:"name"`
		}
		detectedResults := []discoverResult{}
		err = json.Unmarshal(body, &detectedResults)
		if err != nil {
			log.WithError(err).Warn("Issues when parsing discover result")
		}
		if len(detectedResults) < 1 {
			log.Fatal("No conbees found, try manually specifying the URI with -conbee-uri")
		}
		log.Infof("Using conbee \"%s\" at %s:%d for this run", detectedResults[0].Name, detectedResults[0].Internalipaddress, detectedResults[0].Internalport)
		*conbeeURI = fmt.Sprintf("%s:%d", detectedResults[0].Internalipaddress, detectedResults[0].Internalport)
	}

	recordMetrics()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}

var myClient = &http.Client{Timeout: 10 * time.Second}

// Source https://stackoverflow.com/a/31129967
func getJson(url string, target interface{}) error {
	r, err := myClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func getAllSensorStates() error {
	var allSensors map[string]conbeeSensor
	err := getJson(fmt.Sprintf("%s/api/%s/sensors", *conbeeURI, apiKey), &allSensors)

	if err != nil {
		log.WithError(err).Error("Could not get sensors from Conbee!")
	}

	var states map[string]float64
	var labels map[string]string
	for ID, sensor := range allSensors {
		states, labels = getStatesForSensor(sensor)
		labels["Name"] = sensor.Name
		labels["Type"] = sensor.Type
		for key, value := range states {
			log.Debugf("Sensor %s, Measured %s %.2f", sensor.Name, key, value)

			opsQueued := promauto.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: promNamespace,
					Subsystem: ID,
					Name:      key,
				},
				maps.Keys(labels),
			)

			// Increase a value using compact (but order-sensitive!) WithLabelValues().
			opsQueued.With(labels).Set(value)
		}
	}
	// log.Infof("STUFF %+v", states)

	return nil
}

func getStatesForSensor(sensor conbeeSensor) (states map[string]float64, labels map[string]string) {
	states = make(map[string]float64)
	labels = make(map[string]string)

	switch sensor.Type {
	case "ZHAAirQuality":
		labels["Airquality"] = sensor.State.Airquality
		states["Airqualityppb"] = float64(sensor.State.Airqualityppb)
	case "ZHAAlarm":
		states["Alarm"] = Btoi(sensor.State.Alarm)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lowbattery"] = Btoi(sensor.State.Lowbattery)
		states["Tampered"] = Btoi(sensor.State.Tampered)
	case "ZHACarbonMonoxide":
		states["Carbonmonoxide"] = Btoi(sensor.State.Carbonmonoxide)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lowbattery"] = Btoi(sensor.State.Lowbattery)
		states["Tampered"] = Btoi(sensor.State.Tampered)
	case "ZHAConsumption":
		states["Consumption"] = float64(sensor.State.Consumption)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Power"] = float64(sensor.State.Power)
	case "ZHAFire":
		states["Fire"] = Btoi(sensor.State.Fire)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lowbattery"] = Btoi(sensor.State.Lowbattery)
		states["Tampered"] = Btoi(sensor.State.Tampered)
	case "ZHAHumidity":
		states["Humidity"] = float64(sensor.State.Humidity)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
	case "ZHALightLevel":
		states["Lux"] = float64(sensor.State.Lux)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lightlevel"] = float64(sensor.State.Lightlevel)
		states["Dark"] = Btoi(sensor.State.Dark)
		states["Daylight"] = Btoi(sensor.State.Daylight)
	case "ZHAOpenClose":
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lowbattery"] = Btoi(sensor.State.Lowbattery)
		states["Open"] = Btoi(sensor.State.Open)
		states["Tampered"] = Btoi(sensor.State.Tampered)
	case "ZHAPower":
		states["Current"] = float64(sensor.State.Current)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Power"] = float64(sensor.State.Power)
		states["Voltage"] = float64(sensor.State.Voltage)
	case "ZHAPresence":
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lowbattery"] = Btoi(sensor.State.Lowbattery)
		states["Presence"] = Btoi(sensor.State.Presence)
		states["Tampered"] = Btoi(sensor.State.Tampered)
	case "ZHAPressure":
		states["Pressure"] = float64(sensor.State.Pressure)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
	case "ZHASwitch":
		states["Buttonevent"] = float64(sensor.State.Buttonevent)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Gesture"] = float64(sensor.State.Gesture)
		states["Eventduration"] = float64(sensor.State.Eventduration)
		states["X"] = float64(sensor.State.X)
		states["Y"] = float64(sensor.State.Y)
		states["Angle"] = float64(sensor.State.Angle)
	case "ZHATemperature":
		states["Temperature"] = float64(sensor.State.Temperature)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
	case "ZHAThermostat":
		states["On"] = Btoi(sensor.State.On)
		labels["Errorcode"] = fmt.Sprint(sensor.State.Errorcode)
		labels["Fanmode"] = fmt.Sprint(sensor.State.Fanmode)
		states["Floortemperature"] = float64(sensor.State.Floortemperature)
		states["Heating"] = Btoi(sensor.State.Heating)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Mountingmodeactive"] = Btoi(sensor.State.Mountingmodeactive)
		states["Temperature"] = float64(sensor.State.Temperature)
		states["Valve"] = float64(sensor.State.Valve)
		labels["Windowopen"] = fmt.Sprint(sensor.State.Windowopen)
	case "ZHATime":
		labels["Lastset"] = fmt.Sprint(sensor.State.Lastset)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		labels["Localtime"] = fmt.Sprint(sensor.State.Localtime)
		labels["Utc"] = fmt.Sprint(sensor.State.Utc)
	case "ZHAVibration":
		states["Vibration"] = Btoi(sensor.State.Vibration)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		labels["Orientation"] = fmt.Sprint(sensor.State.Orientation)
		states["Tiltangle"] = float64(sensor.State.Tiltangle)
		states["Vibrationstrength"] = float64(sensor.State.Vibrationstrength)
	case "ZHAWater":
		states["Water"] = Btoi(sensor.State.Water)
		labels["Lastupdated"] = fmt.Sprint(sensor.State.Lastupdated)
		states["Lowbattery"] = Btoi(sensor.State.Lowbattery)
		states["Tampered"] = Btoi(sensor.State.Tampered)
	}
	return
}
func Btoi(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
