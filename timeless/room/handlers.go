package room

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/smtp"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hidenkeys/timeless/storage"
	"gorm.io/gorm"
)

// GetAllBookings {params [start, end, employeeId]} // TODO: paging
// get all booking within date range (start and end)

type CustomerDetails struct {
	FirstName *string
	LastName  *string
}

type RoomDetails struct {
	Name     string
	Category string
}

type BookRoomRequeste struct {
	PaymentMethod *string
	Amount        *float64
}

func GetAllBookings(c fiber.Ctx) error {
	//limit := c.Query("page_size", "10")
	//offset := c.Query("page", "0")

	start := c.Query("start")
	end := c.Query("end")
	employeeId := c.Query("employeeId")

	var generateSQL strings.Builder
	generateSQL.WriteString("SELECT * FROM bookings")
	var params []any
	var whereClause strings.Builder

	whereClause.WriteString("deleted_at is ? ")
	params = append(params, nil)

	if start != "" {
		if len(params) != 0 {
			whereClause.WriteString("AND ")
		}
		whereClause.WriteString("created_at >= ? ")
		params = append(params, start)
	}

	if end != "" {
		if len(params) != 0 {
			whereClause.WriteString("AND ")
		}
		whereClause.WriteString("created_at <= ? ")
		params = append(params, fmt.Sprintf("%sT23:59", end))
	}

	if employeeId != "" {
		if len(params) != 0 {
			whereClause.WriteString("AND ")
		}
		whereClause.WriteString("employee_id == ? ")
		params = append(params, employeeId)
	}

	if whereClause.Len() > 0 {
		generateSQL.WriteString(" WHERE ")
		generateSQL.WriteString(whereClause.String())
		generateSQL.WriteString("ORDER BY created_at desc")

	}
	//generateSQL.WriteString(" LIMIT ? OFFSET ?")
	//params = append(params, limit)
	//params = append(params, offset)

	var bookings []Booking
	if result := storage.DB.Preload("RoomBookings").Raw(generateSQL.String(), params...).Find(&bookings); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	return c.Status(http.StatusOK).JSON(bookings)
}

// GetBookingById get booking by booking id
func GetBookingById(c fiber.Ctx) error {
	id := c.Params("id")

	if id == "" {
		return c.Status(http.StatusBadRequest).SendString("invalid booking id")
	}

	var booking Booking

	if result := storage.DB.Preload("RoomBookings").Raw("SELECT * FROM bookings WHERE id == ?", id).Scan(&booking); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	if booking.ID == 0 {
		return c.Status(http.StatusBadRequest).SendString("invalid booking id")
	}

	return c.Status(http.StatusOK).JSON(booking)
}

// ChangePaymentStatus change payment plan
func ChangePaymentStatus(c fiber.Ctx) error {
	id := c.Params("id")
	paymentMethod := c.Query("method")

	if id == "" || paymentMethod == "" {
		return c.Status(http.StatusBadRequest).SendString("invalid booking id")
	}

	if result := storage.DB.Exec("UPDATE bookings SET is_paid = true AND payment_method = ? WHERE id == ?", paymentMethod, id); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	return c.SendStatus(http.StatusOK)
}

type UpdateBookingRequest struct {
	CustomerID      *uint     `json:"customerID" validate:"required"`
	Receptionist    uint      `json:"receptionist"`
	IsPaid          bool      `json:"isPaid"`
	PaymentMethod   string    `json:"paymentMethod" validate:"required"`
	IsComplementary bool      `json:"isComplementary" gorm:"default:false"`
	NumberOfNights  uint      `json:"numberOfNights" validate:"required"`
	StartDate       time.Time `json:"startDate" validate:"required"`
	EndDate         time.Time `json:"endDate" validate:"required"`
	Amount          *float64  `json:"amount"`
	BookingID       uint      `json:"bookingID"`
	RoomID          uint      `json:"roomID"`
}

func UpdateBooking(c fiber.Ctx) error {
	newBookingInfo := new(UpdateBookingRequest)
	bookingID, _ := strconv.Atoi(c.Params("bookingId"))
	roomBookingID, _ := strconv.Atoi(c.Params("roomBookingId"))

	if err := c.Bind().JSON(&newBookingInfo); err != nil {
		return c.Status(http.StatusBadRequest).JSON(err)
	}

	booking := new(Booking)
	booking.ID = uint(bookingID)

	roomBooking := new(RoomBookings)
	roomBooking.ID = uint(roomBookingID)

	var start time.Time
	var _ time.Time

	start = newBookingInfo.StartDate.Add(12 * time.Hour)
	newBookingInfo.EndDate = start.AddDate(0, 0, int(newBookingInfo.NumberOfNights))

	if result := storage.DB.Model(&booking).Updates(newBookingInfo); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed to update booking")
	}

	if result := storage.DB.Model(&roomBooking).Updates(newBookingInfo); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed to update room booking")
	}

	var checkBooking Booking
	if result := storage.DB.Raw("Select * from bookings where id = ?", bookingID).Find(&checkBooking); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed")
	}
	var checkRoomBooking RoomBookings
	if result := storage.DB.Raw("Select * from room_bookings where id = ?", roomBookingID).Find(&checkRoomBooking); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed")
	}

	room_id := checkRoomBooking.RoomID

	var roomDetails Room
	if result := storage.DB.Raw("Select * From rooms where id = ?", room_id).Find(&roomDetails); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed")
	}
	amount := 0.0
	roomBookingsAmount := 0.0
	if newBookingInfo.Amount != nil {
		amount = *newBookingInfo.Amount * float64(newBookingInfo.NumberOfNights)
		roomBookingsAmount = *newBookingInfo.Amount
	} else {
		amount = roomDetails.Price * float64(newBookingInfo.NumberOfNights)
		roomBookingsAmount = roomDetails.Price
	}

	if result := storage.DB.Model(&booking).Update("amount", amount); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed to update booking")
	}

	if result := storage.DB.Model(&roomBooking).Update("amount", roomBookingsAmount); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed to update booking")
	}

	startDate := checkRoomBooking.StartDate
	endDate := checkRoomBooking.EndDate
	noOfNights := checkRoomBooking.NumberOfNights

	customerID := *checkBooking.CustomerID
	var customerDetails Customer

	//if result := storage.DB.Raw("SELECT * from customer where id = ?", customerID).Find(&customerDetails); result.Error != nil {
	//	return result.Error
	//}
	storage.DB.First(&customerDetails, "id = ?", customerID)

	receipt := Receipt{
		CustomerName:    fmt.Sprintf("%s %s", *customerDetails.LastName, *customerDetails.FirstName),
		ReceiptNumber:   generateReceiptNumber(),
		TransactionDate: time.Now().Format("2006-01-02"),
		PaymentMethod:   checkBooking.PaymentMethod,
		AmountPaid:      *checkBooking.Amount,
		StartDate:       startDate.Format("2006-01-02"),
		EndDate:         endDate.Format("2006-01-02"),
		NumberOfNights:  int(noOfNights),
		LogoURL:         "https://res.cloudinary.com/dzi8kxyze/image/upload/v1724162969/qclo5v2qhzra7sxmda05.png", // Provide the URL to your logo here

	}

	tmpl, err := template.ParseFiles("room/update.html")
	if err != nil {
		log.Fatalf("Error loading template: %v", err)
	}

	// Parse the template with the receipt data
	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, receipt); err != nil {
		log.Fatalf("Error executing template: %v", err)
	}

	// Convert parsed template to a string
	receiptBody := tpl.String()

	// Send the email
	go sendEmail(*customerDetails.Email, "Your Payment Receipt", receiptBody)

	printerName := "POS-80-test1"
	guestName := *customerDetails.LastName + " " + *customerDetails.FirstName
	roomType := *roomDetails.Name
	//roomCategory := roomDetails.Category
	//PaymentMethod := bookRoomRequest.PaymentMethod
	//checkInDate := startDate
	//checkOutDate := endDate
	//totalAmount := *bookRoomRequest.Amount

	checkInDate := startDate.Format("2006-01-02")
	checkOutDate := endDate.Format("2006-01-02")
	totalyAmount := *checkBooking.Amount

	cmd := exec.Command("python", "hotel_receipt.py", printerName, guestName, roomType, checkInDate, checkOutDate, fmt.Sprintf("%.2f", totalyAmount))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		fmt.Printf("Detailed Output: %s\n", string(output)) // Capture detailed output from the Python script
		return nil
	}

	fmt.Printf("Output: %s\n", output)

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"message":     "Booking and Room Booking updated successfully",
		"booking":     booking,
		"roomBooking": roomBooking,
	})
}

func CheckIn(c fiber.Ctx) error {
	roomBookingId, err := strconv.Atoi(c.Params("id"))

	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fmt.Errorf("invalid customer id"))
	}

	roomBooking := new(RoomBookings)
	roomBooking.ID = uint(roomBookingId)

	updates := map[string]interface{}{
		"CheckedIn":  true,
		"CheckedOut": false,
	}

	if result := storage.DB.Model(RoomBookings{}).Where("id = ?", roomBookingId).Updates(updates); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	var updatedRoomBooking RoomBookings

	if result := storage.DB.Raw("SELECT * FROM room_bookings WHERE id = ?", roomBookingId).Find(&updatedRoomBooking); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	roomUpdates := map[string]interface{}{
		"Status": "Unavailable",
	}

	roomId := updatedRoomBooking.RoomID

	if result := storage.DB.Model(Room{}).Where("id = ?", roomId).Updates(roomUpdates); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	return c.Status(http.StatusOK).JSON(updatedRoomBooking)
}

func CheckOut(c fiber.Ctx) error {
	roomBookingId, err := strconv.Atoi(c.Params("id"))

	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fmt.Errorf("invalid customer id"))
	}

	roomBooking := new(RoomBookings)
	roomBooking.ID = uint(roomBookingId)

	updates := map[string]interface{}{
		"CheckedIn":  false,
		"CheckedOut": true,
	}

	if result := storage.DB.Model(RoomBookings{}).Where("id = ?", roomBookingId).Updates(updates); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	var updatedRoomBooking RoomBookings

	if result := storage.DB.Raw("SELECT * FROM room_bookings WHERE id = ?", roomBookingId).Find(&updatedRoomBooking); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	roomUpdates := map[string]interface{}{
		"Status": "available",
	}

	roomId := updatedRoomBooking.RoomID

	if result := storage.DB.Model(Room{}).Where("id = ?", roomId).Updates(roomUpdates); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	return c.Status(http.StatusOK).JSON(updatedRoomBooking)
}

func ViewSingleRoomBooking(c fiber.Ctx) error {
	bookingID := c.Params("bookingId")
	roomBookingID := c.Params("roomBookingId")

	if bookingID == "" {
		return c.Status(http.StatusInternalServerError).SendString("booking id not found")
	}

	if roomBookingID == "" {
		return c.Status(http.StatusInternalServerError).SendString("roomBooking id not found")
	}
	var roomBooking RoomBookings
	if result := storage.DB.Where("booking_id = ? AND id = ?", bookingID, roomBookingID).First(&roomBooking); result.Error != nil {
		return c.Status(http.StatusInternalServerError).SendString("can't get room booking")
	}

	return c.Status(http.StatusOK).JSON(roomBooking)
}

func DeleteBooking(c fiber.Ctx) error {
	id := c.Params("id")

	if id == "" {
		return c.Status(http.StatusBadRequest).SendString("invalid booking id")
	}

	tx := storage.DB.Begin()

	// Check and delete all room bookings associated with the booking
	if result := tx.Unscoped().Where("booking_id = ?", id).Delete(&RoomBookings{}); result.Error != nil {
		tx.Rollback() // Rollback in case of error
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	// Delete the main booking entry
	if result := tx.Unscoped().Where("id = ?", id).Delete(&Booking{}); result.Error != nil {
		tx.Rollback() // Rollback in case of error
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(err)
	}

	return c.SendStatus(http.StatusNoContent)
}

// GetBookingSummary {params [start, end]}
// get booking summary for a particular date range (money_made, no_of_bookings, check_in, check_out, no_of_available_rooms, by_cash, by_pos, by_transfer)
func GetBookingSummary(c fiber.Ctx) error {

	start := c.Query("start")
	end := c.Query("end")

	var params []any
	var whereClause strings.Builder

	whereClause.WriteString("deleted_at is ? ")
	params = append(params, nil)

	if start != "" {
		if len(params) != 0 {
			whereClause.WriteString("AND ")
		}
		whereClause.WriteString("created_at >= ? ")
		params = append(params, start)
	}

	if end != "" {
		if len(params) != 0 {
			whereClause.WriteString("AND ")
		}
		whereClause.WriteString("created_at <= ? ")
		params = append(params, end)
	}

	sqlString := getSummaryQuery
	if whereClause.Len() > 0 {
		sqlString = strings.Replace(sqlString,
			"select * from bookings",
			fmt.Sprintf("select * from bookings where %s ", whereClause.String()),
			1)
	}

	var sumAmount, numberOfBookings, sumAmountCash, sumAmountPos, sumAmountTransfer float64
	var checkIn, checkOut, availableRooms uint

	row := storage.DB.Raw(sqlString, params...).Row()
	err := row.Scan(&sumAmount, &numberOfBookings, &sumAmountCash, &sumAmountPos, &sumAmountTransfer, &checkIn, &checkOut, &availableRooms)
	if err != nil {
		log.Println(err)
		return c.Status(http.StatusInternalServerError).JSON(err)
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"sumAmount":         sumAmount,
		"numberOfBookings":  numberOfBookings,
		"sumAmountCash":     sumAmountCash,
		"sumAmountPos":      sumAmountPos,
		"sumAmountTransfer": sumAmountTransfer,
		"checkIn":           checkIn,
		"checkOut":          checkOut,
	})
}

// function to send email
func sendEmail(to, subject, body string) {
	from := "teniolasobande04@gmail.com"
	password := "vndt vleo ccfc tcqt"

	// Set up authentication information.
	auth := smtp.PlainAuth("", from, password, "smtp.gmail.com")

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: " + subject + "\n" +
		"MIME-version: 1.0;\n" +
		"Content-Type: text/html; charset=\"UTF-8\";\n\n" +
		body

	err := smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, []byte(msg))
	if err != nil {
		log.Fatalf("Error sending email: %v", err)
	}
}

func generateReceiptNumber() string {
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		log.Fatalf("Error generating random bytes: %v", err)
	}
	randomPart := fmt.Sprintf("%x", randomBytes)
	return fmt.Sprintf("%d-%s", timestamp, randomPart)
}

type Receipt struct {
	CustomerName    string
	ReceiptNumber   string
	TransactionDate string
	PaymentMethod   string
	AmountPaid      float64
	LogoURL         string
	StartDate       string
	EndDate         string
	NumberOfNights  int
}

// BookingRoom book a room
func BookRoom(c fiber.Ctx) error {
	bookRoomRequest := new(Booking)

	if err := c.Bind().JSON(bookRoomRequest); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(err)
	}

	totalAmount := 0.0

	startDate := ""
	endDate := ""
	noOfNights := 0
	roomNumber := 0

	// check if the scheduled booking doesn't clash with another room booking
	for _, roomBooking := range bookRoomRequest.RoomBookings {
		// find room by id
		r := Room{
			Model: gorm.Model{
				ID: roomBooking.RoomID,
			},
		}

		if result := storage.DB.Find(&r); result.Error != nil {
			return c.Status(http.StatusInternalServerError).JSON(result.Error)
		}

		if r.ID == 0 {
			return c.Status(http.StatusInternalServerError).SendString("invalid room id")
		}

		var start time.Time
		var end time.Time
		var err error

		if roomBooking.StartDate.IsZero() {
			start, err = time.Parse(time.DateOnly, time.Now().UTC().Format(time.DateOnly))
			start = start.Add(12 * time.Hour)
			end = start.AddDate(0, 0, int(roomBooking.NumberOfNights))
		} else {
			start = roomBooking.StartDate.Add(12 * time.Hour)
			end = start.AddDate(0, 0, int(roomBooking.NumberOfNights))
		}

		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(err)
		}

		// get the individual dates
		nights := []time.Time{start.Add(-12 * time.Hour)}

		for i := uint(1); i < roomBooking.NumberOfNights; i++ {
			nights = append(nights, start.AddDate(0, 0, int(i)))
		}

		for _, night := range nights {
			dates, err1 := getBookedDatesByRoomID(roomBooking.RoomID)
			if err1 != nil {
				return c.Status(http.StatusInternalServerError).SendString(err1.Error())
			}

			if slices.Contains(dates, night) {
				year, month, day := night.Date()
				return c.Status(http.StatusBadRequest).SendString(fmt.Sprintf("room number %s is booked on %d/%d/%d", *r.Name, day, month, year))
			}
		}

		startDate = start.Format("2006-01-02")
		endDate = end.Format("2006-01-02")
		noOfNights = int(roomBooking.NumberOfNights)

		// Check for overlaps with existing bookings
		existingBookings, err := getBookedDatesByRoomID(roomBooking.RoomID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).SendString(err.Error())
		}

		for _, existingBookingDate := range existingBookings {
			if (start.Before(existingBookingDate.Add(24*time.Hour)) && end.After(existingBookingDate)) ||
				(end.After(existingBookingDate) && start.Before(existingBookingDate.Add(24*time.Hour))) {
				year, month, day := existingBookingDate.Date()
				return c.Status(http.StatusBadRequest).SendString(fmt.Sprintf("room number %s is booked on %d/%d/%d", *r.Name, day, month, year))
			}
		}

		roomBooking.StartDate = start
		roomBooking.EndDate = end
		roomNumber = int(roomBooking.RoomID)

		// i.e is null
		if roomBooking.Amount == nil {
			a := r.Price
			roomBooking.Amount = &a
		}

		totalAmount += *roomBooking.Amount * float64(roomBooking.NumberOfNights)
	}

	bookRoomRequest.Amount = &totalAmount

	if result := storage.DB.Create(bookRoomRequest); result.Error != nil {
		return c.Status(http.StatusInternalServerError).JSON(result.Error)
	}

	customerID := *bookRoomRequest.CustomerID
	var customerDetails Customer
	var roomDetails Room

	//if result := storage.DB.Raw("SELECT * from customer where id = ?", customerID).Find(&customerDetails); result.Error != nil {
	//	return result.Error
	//}
	storage.DB.First(&customerDetails, "id = ?", customerID)
	storage.DB.First(&roomDetails, "id = ?", roomNumber)

	receipt := Receipt{
		CustomerName:    fmt.Sprintf("%s %s", *customerDetails.LastName, *customerDetails.FirstName),
		ReceiptNumber:   generateReceiptNumber(),
		TransactionDate: time.Now().Format("2006-01-02"),
		PaymentMethod:   bookRoomRequest.PaymentMethod,
		AmountPaid:      *bookRoomRequest.Amount,
		StartDate:       startDate,
		EndDate:         endDate,
		NumberOfNights:  noOfNights,
		LogoURL:         "https://res.cloudinary.com/dzi8kxyze/image/upload/v1724162969/qclo5v2qhzra7sxmda05.png", // Provide the URL to your logo here

	}

	tmpl, err := template.ParseFiles("room/reciept.html")
	if err != nil {
		log.Fatalf("Error loading template: %v", err)
	}

	// Parse the template with the receipt data
	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, receipt); err != nil {
		log.Fatalf("Error executing template: %v", err)
	}

	// Convert parsed template to a string
	receiptBody := tpl.String()

	// Send the email
	go sendEmail(*customerDetails.Email, "Your Payment Receipt", receiptBody)

	printerName := "POS-80-test1"
	guestName := *customerDetails.LastName + " " + *customerDetails.FirstName
	roomType := *roomDetails.Name
	//roomCategory := roomDetails.Category
	//PaymentMethod := bookRoomRequest.PaymentMethod
	//checkInDate := startDate
	//checkOutDate := endDate
	//totalAmount := *bookRoomRequest.Amount

	checkInDate := startDate
	checkOutDate := endDate
	totalyAmount := *bookRoomRequest.Amount

	cmd := exec.Command("python", "hotel_receipt.py", printerName, guestName, roomType, checkInDate, checkOutDate, fmt.Sprintf("%.2f", totalyAmount))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		fmt.Printf("Detailed Output: %s\n", string(output)) // Capture detailed output from the Python script
		return nil
	}

	fmt.Printf("Output: %s\n", output)

	return c.Status(http.StatusOK).JSON(*bookRoomRequest)
}

// BookRoom book a room
// func BookRoom(c fiber.Ctx) error {
// 	bookRoomRequest := new(Booking)

// 	if err := c.Bind().JSON(bookRoomRequest); err != nil {
// 		return c.Status(http.StatusInternalServerError).JSON(err)
// 	}

// 	totalAmount := 0.0

// 	// check if the scheduled booking doesn't clash with another room booking
// 	for _, roomBooking := range bookRoomRequest.RoomBookings {
// 		// find room by id
// 		r := Room{
// 			Model: gorm.Model{
// 				ID: roomBooking.RoomID,
// 			},
// 		}

// 		if result := storage.DB.Find(&r); result.Error != nil {
// 			return c.Status(http.StatusInternalServerError).JSON(result.Error)
// 		}

// 		if r.ID == 0 {
// 			return c.Status(http.StatusInternalServerError).SendString("invalid room id")
// 		}

// 		var start time.Time
// 		var end time.Time
// 		var err error

// 		if roomBooking.StartDate.IsZero() {
// 			start, err = time.Parse(time.DateOnly, time.Now().UTC().Format(time.DateOnly))
// 			start = start.Add(12 * time.Hour)
// 			end = start.AddDate(0, 0, int(roomBooking.NumberOfNights))
// 		} else {
// 			start = roomBooking.StartDate.Add(12 * time.Hour)
// 			end = start.AddDate(0, 0, int(roomBooking.NumberOfNights))
// 		}

// 		if err != nil {
// 			return c.Status(http.StatusInternalServerError).JSON(err)
// 		}

// 		// get the individual dates
// 		nights := []time.Time{start.Add(-12 * time.Hour)}

// 		for i := uint(1); i < roomBooking.NumberOfNights; i++ {
// 			nights = append(nights, start.AddDate(0, 0, int(i)))
// 		}

// 		for _, night := range nights {
// 			dates, err1 := getBookedDatesByRoomID(roomBooking.RoomID)
// 			if err1 != nil {
// 				return c.Status(http.StatusInternalServerError).SendString(err1.Error())
// 			}

// 			if slices.Contains(dates, night) {
// 				year, month, day := night.Date()
// 				return c.Status(http.StatusBadRequest).SendString(fmt.Sprintf("room number %s is booked on %d/%d/%d", *r.Name, day, month, year))
// 			}
// 		}

// 		roomBooking.StartDate = start
// 		roomBooking.EndDate = end

// 		// i.e is null
// 		if roomBooking.Amount == nil {
// 			a := r.Price
// 			roomBooking.Amount = &a
// 		}

// 		totalAmount += *roomBooking.Amount * float64(roomBooking.NumberOfNights)
// 	}

// 	bookRoomRequest.Amount = &totalAmount

// 	if result := storage.DB.Create(bookRoomRequest); result.Error != nil {
// 		return c.Status(http.StatusInternalServerError).JSON(result.Error)
// 	}

// 	return c.Status(http.StatusOK).JSON(*bookRoomRequest)
// }

func GetBookedDates(c fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))

	if err != nil {
		return c.Status(http.StatusBadRequest).SendString("invalid room id")
	}

	dates, err := getBookedDatesByRoomID(uint(id))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(err)
	}

	return c.Status(http.StatusOK).JSON(dates)
}

func getBookedDatesByRoomID(roomID uint) ([]time.Time, error) {
	rows, err := storage.DB.Raw(getBookedDatesByRoomIDQuery, roomID).Rows()
	if err != nil {
		return []time.Time{}, err
	}
	defer rows.Close()

	var bookedDates []time.Time
	for rows.Next() {
		var a string
		err1 := rows.Scan(&a)
		if err1 != nil {
			return []time.Time{}, err1
		}

		if b, err2 := time.Parse(time.DateOnly, a); err2 == nil {
			bookedDates = append(bookedDates, b)
		}
	}

	return bookedDates, nil
}

type BookRoomRequest struct {
	CustomerID      *uint  `json:"customerID"`
	Receptionist    uint   `json:"receptionist"`
	IsPaid          bool   `json:"isPaid"`
	PaymentMethod   string `json:"paymentMethod"`
	IsComplementary bool   `json:"isComplementary" gorm:"default:false"`
	RoomBookings    []struct {
		NumberOfNights uint      `json:"numberOfNights"`
		RoomID         uint      `json:"roomID"`
		Amount         *float64  `json:"amount"`
		StartDate      time.Time `json:"startDate"`
	} `json:"roomBookings"`
}

const (
	getSummaryQuery = `
	with b1 as (
    	select * from bookings
	)

	select 
    	(
        	select coalesce(sum(amount),0) from b1
    	) as sum_amount,
    	(
        	select count(amount) from b1
    	) as  no_of_bookings,
    	(
        	select coalesce(sum(amount),0) from b1 where payment_method == 'Cash'
        ) as sum_amount_cash,
    	(
        	select coalesce(sum(amount),0) from b1 where payment_method == 'Credit Card'
    	) as sum_amount_pos,
    	(
        	select coalesce(sum(amount),0) from b1 where payment_method == 'Transfer'
    	) as sum_amount_transfer,
    	(
        	select count(*) from b1 left join room_bookings on room_bookings.booking_id =b1.id  where checked_in is true
    	) as num_check_ins_today,
    	(
        	select count(*) from b1 left join room_bookings on room_bookings.booking_id =b1.id  where checked_out is true
    	) as num_check_outs_today,
    	(
        	select count(*) from rooms where id not in (
            	select room_id from room_bookings where start_date <= datetime() and end_date >= datetime() and checked_in is true
            	)
    	) as num_available_rooms_today
	`

	getBookedDatesByRoomIDQuery = `
	with recursive list(d1, d2, num_nights) as (
    select
        date(start_date) as d1,
        date(end_date) as d2,
        1 AS num_nights
    from room_bookings where checked_out is false and room_id == ?
    union
    select
        date(d1, format('+%d days', 1)),
        d2,
        num_nights + 1 from list
    where date(d1, format('+%d days', 1)) < d2
	)

	select d1 as booked_dates from list;
	`
)

// const (
// 	getSummaryQuery = `
// 	with b1 as (
//     	select * from bookings
// 	)

// 	select
//     	(
//         	select coalesce(sum(amount),0) from b1
//     	) as sum_amount,
//     	(
//         	select count(amount) from b1
//     	) as  no_of_bookings,
//     	(
//         	select coalesce(sum(amount),0) from b1 where payment_method == 'Cash'
//         ) as sum_amount_cash,
//     	(
//         	select coalesce(sum(amount),0) from b1 where payment_method == 'Credit Card'
//     	) as sum_amount_pos,
//     	(
//         	select coalesce(sum(amount),0) from b1 where payment_method == 'Transfer'
//     	) as sum_amount_transfer,
//     	(
//         	select count(*) from room_bookings where start_date <= datetime() and end_date >= datetime() and checked_in is true
//     	) as num_check_ins_today,
//     	(
//         	select count(*) from room_bookings where date(end_date) == date() and checked_out is true
//     	) as num_check_outs_today,
//     	(
//         	select count(*) from rooms where id not in (
//             	select room_id from room_bookings where start_date <= datetime() and end_date >= datetime() and checked_in is true
//             	)
//     	) as num_available_rooms_today
// 	`

// 	getBookedDatesByRoomIDQuery = `
// 	with recursive list(d1, d2, num_nights) as (
//     select
//         date(start_date) as d1,
//         date(end_date) as d2,
//         1 AS num_nights
//     from room_bookings where checked_out is false and room_id == ?
//     union
//     select
//         date(d1, format('+%d days', 1)),
//         d2,
//         num_nights + 1 from list
//     where date(d1, format('+%d days', 1)) < d2
// 	)

// 	select d1 as booked_dates from list;
// 	`
// )
