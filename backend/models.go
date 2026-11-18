package main

import (
	"math"
	"time"
)

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

type Driver struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	Phone             string     `json:"phone"`
	VehicleModel      string     `json:"vehicle_model"`
	VehiclePlate      string     `json:"vehicle_plate"`
	Available         bool       `json:"available"`
	Lat               *float64   `json:"lat"`
	Lng               *float64   `json:"lng"`
	LocationUpdatedAt *time.Time `json:"location_updated_at"`
	Rating            *float64   `json:"rating"` // average, null until rated
	RatingCount       int        `json:"rating_count"`
	CreatedAt         time.Time  `json:"created_at"`
}

// NearbyDriver is the limited public view of a driver shown to riders.
type NearbyDriver struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	VehicleModel string   `json:"vehicle_model"`
	VehiclePlate string   `json:"vehicle_plate"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
	Rating       *float64 `json:"rating"`
	DistanceKm   float64  `json:"distance_km"`
}

type Ride struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	RiderName     string    `json:"rider_name"`
	DriverID      *int64    `json:"driver_id"`
	DriverName    *string   `json:"driver_name"`
	DriverVehicle *string   `json:"driver_vehicle"`
	DriverPlate   *string   `json:"driver_plate"`
	DriverPhone   *string   `json:"driver_phone"`
	DriverLat     *float64  `json:"driver_lat"` // live location, for tracking
	DriverLng     *float64  `json:"driver_lng"`
	Pickup        string    `json:"pickup"`
	Dropoff       string    `json:"dropoff"`
	PickupLat     float64   `json:"pickup_lat"`
	PickupLng     float64   `json:"pickup_lng"`
	DropoffLat    float64   `json:"dropoff_lat"`
	DropoffLng    float64   `json:"dropoff_lng"`
	DistanceKm    float64   `json:"distance_km"`
	Fare          float64   `json:"fare"`
	Status        string    `json:"status"`
	PaymentMethod string    `json:"payment_method"`
	PaymentStatus string    `json:"payment_status"`
	Rating        *int      `json:"rating"`
	Review        *string   `json:"review"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const (
	RoleRider  = "rider"
	RoleDriver = "driver"

	StatusRequested = "requested"
	StatusAccepted  = "accepted"
	StatusStarted   = "started"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"

	PayPending = "pending"
	PayPaid    = "paid"
)

var paymentMethods = map[string]bool{"cash": true, "card": true, "wallet": true}

// Allowed status transitions.
var transitions = map[string][]string{
	StatusRequested: {StatusAccepted, StatusCancelled},
	StatusAccepted:  {StatusStarted, StatusCancelled},
	StatusStarted:   {StatusCompleted},
}

func canTransition(from, to string) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Fare configuration.
const (
	baseFare   = 30.0
	perKmRate  = 12.0
	minimumFee = 50.0
)

// haversineKm returns the great-circle distance between two points in km.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func calcFare(distanceKm float64) float64 {
	fare := baseFare + perKmRate*distanceKm
	if fare < minimumFee {
		fare = minimumFee
	}
	return math.Round(fare*100) / 100
}
