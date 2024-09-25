import sys
from datetime import datetime
import win32print
import win32ui
from PIL import Image, ImageWin
import requests
from io import BytesIO

def print_hotel_receipt(printer_name, guest_name, room_type, check_in_date, check_out_date, total_amount):
    try:
        # Get current time
        now = datetime.now()
        date = now.strftime("%m/%d/%Y")
        time = now.strftime("%I:%M %p")

        # Open printer
        hPrinter = win32print.OpenPrinter(printer_name)
        hDC = win32ui.CreateDC()
        hDC.CreatePrinterDC(printer_name)
        hDC.StartDoc("Hotel Booking Receipt")
        hDC.StartPage()

        # Load hotel logo
        logo_url = "https://res.cloudinary.com/dzi8kxyze/image/upload/v1724162969/qclo5v2qhzra7sxmda05.png"
        response = requests.get(logo_url)
        if response.status_code != 200:
            print(f"Error: Unable to fetch logo from {logo_url}. HTTP Status Code: {response.status_code}")
            sys.exit(1)

        # Open and convert the image to RGB
        logo_image = Image.open(BytesIO(response.content))
        logo_image = logo_image.convert("RGB")  # Ensure image is in RGB format

        # Convert PIL Image to a format that can be printed
        dib = ImageWin.Dib(logo_image)

        # Define image size and position
        x = 100  # Starting x position
        y = 100  # Starting y position (top of the page)
        image_width = 300  # Adjust width as needed
        image_height = int(logo_image.height * (image_width / logo_image.width))  # Scale height proportionally

        # Draw the image at the top
        dib.draw(hDC.GetHandleOutput(), (x, y, x + image_width, y + image_height))

        # Move the starting y position down to avoid overlapping with the image
        y += image_height + 20  # Adding some space below the image

        # Set font for text
        font = win32ui.CreateFont({
            "name": "Arial",
            "height": 24,
            "weight": 400,
        })
        hDC.SelectObject(font)

        # Print hotel info below the image
        hDC.TextOut(x, y, "TIMELESS APARTMENTS AND BAR")
        y += 30
        hDC.TextOut(x, y, "62 LANDBRIDGE AVENUE")
        y += 30
        hDC.TextOut(x, y, "ONIRU, LAGOS STATE")
        y += 30

        # Print receipt details
        hDC.TextOut(x, y, f"Date: {date}")
        y += 30
        hDC.TextOut(x, y, f"Time: {time}")
        y += 30
        hDC.TextOut(x, y, f"Guest Name: {guest_name}")
        y += 30
        hDC.TextOut(x, y, f"Room Type: {room_type}")
        y += 30
        hDC.TextOut(x, y, f"Check-in Date: {check_in_date}")
        y += 30
        hDC.TextOut(x, y, f"Check-out Date: {check_out_date}")
        y += 30
        hDC.TextOut(x, y, "-" * 30)
        y += 30
        hDC.TextOut(x, y, f"Total Amount: ${total_amount:.2f}")
        y += 30

        # Print thank you note
        hDC.TextOut(x, y, "Thank you for staying with us!")

        hDC.EndPage()
        hDC.EndDoc()

    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

    finally:
        win32print.ClosePrinter(hPrinter)

if __name__ == "__main__":
    if len(sys.argv) == 7:
        print_hotel_receipt(
            printer_name=sys.argv[1],
            guest_name=sys.argv[2],
            room_type=sys.argv[3],
            check_in_date=sys.argv[4],
            check_out_date=sys.argv[5],
            total_amount=float(sys.argv[6])
        )
    else:
        print("Usage: python hotel_receipt.py <printer_name> <guest_name> <room_type> <check_in_date> <check_out_date> <total_amount>")
