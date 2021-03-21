# BC Robotics Relay HAT
control Raspberry Pi 4 Channel Relay HAT and Raspberry Pi Zero Relay HAT

## Description Raspberry Pi Zero Relay HAT
With the addition of WiFi and Bluetooth to the Raspberry Pi Zero W, it is now finding itself in many more IoT applications. This 2 Channel Relay HAT makes driving higher current and higher voltage devices as easy as possible! This premium Relay HAT matches the Raspberry Pi Zero form factor and is compatible with all versions of the Pi Zero. Each 10A relay has the Input, Normally Open, and Normally Closed contact broken out to a nice 5mm pitch screw terminal. The board uses high quality North American sourced parts, a locally produced circuit board, and simple logic level inputs.

This HAT is compatible with the Raspberry Pi Zero / Zero 1.3 / Zero W and uses GPIO pins 4 and 17 (Pins 7 & 11) on the GPIO header. Each relay driver is connected to the GPIO through a solder jumper and can be alternatively connected through the 2 pin 0.100″ header if a custom configuration is required.

This board does not ship with a header – we recommend the GPIO Header for Raspberry Pi HAT. Standoffs, like those included in our HAT hardware kit , are ideal for ensuring everything stays in place!

Please Note: While this board is capable of switching higher voltages, please exercise caution. If you are inexperienced or unsure about how to use this product safely we recommend looking at the IoT Power Relay, which has all of its high voltage circuitry fully enclosed.

### Features
Fits directly on the Pi's 40 pin GPIO header
Uses GPIO 4 and GPIO 17
Switch up to 10A per channel!
Compatible with all models of the Raspberry Pi Zero

## Description Raspberry Pi 4 Channel Relay HAT
Need to drive high current or high voltage devices with your Raspberry Pi? This premium 4 channel 10A relay HAT can handle it! Each relay has the Input, Normally Open, and Normally Closed contact broken out to a nice 5mm pitch screw terminal. This HAT is compatible with the Raspberry Pi A+/B+/2/3B/3+/4 and uses GPIO pins 4, 17, 27, and 22 (Pins 7,11,13,15) on the GPIO header. Each relay driver is connected to the GPIO through a solder jumper and can be alternatively connected through the 4 pin 0.100″ header if a custom configuration is required.

This board does not ship with a header so minor soldering will be required before it can be used with the Pi. We recommend a Tall GPIO Header for this board.

As of September 14th 2017 we are now shipping version 1.1 of this board. Version 1.1 adds isolation routing to the board and a few SMD components have been shifted around. It is otherwise functionally identical. This board Works with all “A” and “B” versions of the Raspberry Pi (Pi A+, B+, 2, 3, 3A+, 3B+, and Pi 4)

Please Note: While this board is capable of switching higher voltages, please exercise caution. If you are inexperienced or unsure about how to use this product safely we recommend looking at the IoT Power Relay, which has all of its high voltage circuitry fully enclosed.

### Features
Fits directly on the Pi's 40 pin GPIO header
Uses GPIO 4,17,27,22
Switch up to 10A per channel!
Compatible with all models of the Raspberry Pi Zero


## Getting Started With The Raspberry Pi Relay HAT
Our Pi Relay HATs are designed to allow your Pi to switch higher voltages and higher currents from one self contained board. In this tutorial we are going to go over soldering the header to the Relay HAT, use Python with the included Pi.GPIO library to write code that triggers each relay, and go over the external relay connections and configuration options on the board

About The Boards:
We make two versions of this relay board, one for the standard Raspberry Pi with 4 relays, and one for the Raspberry Pi Zero with 2 relays. On the board, each relay’s Common, Normally Open, and Normally Closed pins are brought out to screw terminals. These are not the most sophisticated circuits, but they do provide a compact, permanent solution for attaching a number of relays to the Pi.

## A Quick Overview
There isn’t much in the way of assembly required with these boards as they ship with everything but a header installed. We do not solder a header to the board as different heights or types may be required depending on your application (and removing them can be quite a pain!).

For all standard Raspberry Pi we recommend using a Tall Header, as this will allow inputs on the “USB / Ethernet” side of the Pi to clear. For the Pi Zero, a shorter header can be used so we recommend using the standard GPIO header, but feel free to go a different way as needed. Once we have the headers soldered in, we will use Python and the GPIO library to write some code to trigger each relay, and finally we will look at the different connections on the board.


https://bc-robotics.com/shop/raspberry-pi-zero-relay-hat/

https://bc-robotics.com/shop/raspberry-pi-4-channel-relay-hat/

https://bc-robotics.com/tutorials/getting-started-raspberry-pi-relay-hat/
