package main

import (
	"fmt"
	"gorm.io/gorm"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/hidenkeys/timeless/room"
	"github.com/hidenkeys/timeless/storage"
	"github.com/hidenkeys/timeless/user"
)

func main() {
	db, err := storage.ConnectDB()
	if err != nil {
		log.Fatal(err)
	}
	err = db.AutoMigrate(&user.User{}, &user.Image{}, &room.Booking{}, &room.RoomBookings{}, &room.Customer{}, &room.Room{})
	if err != nil {
		log.Fatal(err)
	}
	err = seedDB(db)
	if err != nil {
		log.Fatal(err)
	}
	app := fiber.New(fiber.Config{AppName: "TIMELESS"})

	app.Use(cors.New(cors.Config{
		//AllowOrigins: "http://localhost:3000", // Allow requests from this origin
		AllowOrigins:     "http://127.0.0.1:5173", // Frontend origin
		AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept",
		AllowCredentials: true,
	}))

	app.Static("/", "./dist")

	api := app.Group("/api/v1")

	bookingsApi := api.Group("/bookings")
	usersApi := api.Group("/users")
	roomsApi := api.Group("/rooms")
	customersApi := api.Group("/customers")

	bookingRoutes(bookingsApi)
	userRoutes(usersApi)
	roomRoutes(roomsApi)
	customerRoutes(customersApi)

	// Set up a wildcard route to serve the index.html for all routes not matching an API route
	app.Get("/*", func(c fiber.Ctx) error {
		return c.SendFile("./dist/index.html")
	})

	err = app.Listen(":3000")
	if err != nil {
		log.Fatal(err)
	}
}

func seedDB(db *gorm.DB) error {
	email := "admin@timeless.com"
	var existingUser user.User
	result := db.Where("email = ?", email).First(&existingUser)

	if result.Error == nil {
		return nil
	}

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return result.Error
	}

	firstName := "Admin"

	newUser := user.User{
		FirstName: &firstName,
		LastName:  &firstName,
		Email:     &email,
		IsAdmin:   true, Password: "superadminpass111",
		Role: "owner",
	}
	if err := db.Create(&newUser).Error; err != nil {
		return err
	}

	fmt.Println("Admin Seeded successfully")
	return nil
}
