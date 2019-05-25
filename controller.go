package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/tarm/serial"
)

const sleepDuration = 600

var modeFlag = new(bool)
var switcher = make(chan bool)
var exitCh = make(chan bool)
var strBuilder = new(strings.Builder)
var port = flag.String("port", "COM6", "Specify serial port")

func platformInfo() {
	var platform, family, version, _ = host.PlatformInformation()
	fmt.Printf("Platform: %s, Family: %s, Version: %s\n", platform, family, version)
}

func adjustSpeed() int {
	// 计算每个cpu核心利用率，duty = max(40 - 0.8 * utilization, 5)
	var utilization float64
	cpuNum, _ := cpu.Counts(true)
	utilizationSlice, _ := cpu.Percent(time.Millisecond*sleepDuration, true)
	for _, value := range utilizationSlice {
		utilization += (value)
	}
	utilization = math.Min(100, math.Max(0, math.Ceil(utilization/float64(cpuNum))))
	return int(math.Max(40.0-0.8*utilization, 5))
}

func writeCommand(serialCom *serial.Port, command string) {
	writtenNum, err := serialCom.Write([]byte(command))
	if err != nil {
		log.Fatalf("[ERROR] An error occurred when write coammand to device: %s", err)
	}
	fmt.Printf("[INFO] Written %d bytes\n", writtenNum)
}

func wrapCommand(commandStr string) string {
	strBuilder.Reset()
	var strArray = [...]string{"N,2#", commandStr, ";"}
	for _, value := range strArray {
		strBuilder.WriteString(value)
	}
	return strBuilder.String()
}

func autoMode(serialCom *serial.Port) {
	for {
		if *modeFlag {
			speed := int64(adjustSpeed())
			_, err := serialCom.Write([]byte(wrapCommand(strconv.FormatInt(speed, 10))))
			if err != nil {
				log.Fatalf("[ERROR] An error occurred when write coammand to device: %s", err)
			}
			time.Sleep(1 * time.Second)
		} else {
			// 阻塞，等待运行时机
			<-switcher
		}
	}
}

func managedMode(serialCom *serial.Port, reader *bufio.Reader) {
	for {
		fmt.Print("Command -->:")
		command, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalln("[ERROR] An error occurred when read string from console !")
		}
		fmt.Printf("Received command: %s", command)
		lowerCommand := strings.ToLower(strings.Trim(command, "\r\n.( )"))
		if strings.Contains("exitquit", lowerCommand) {
			if *modeFlag {
				fmt.Println("Currently running on auto mode, please use \"cancel\" command switch to managed mode firstly")
			} else {
				writeCommand(serialCom, wrapCommand("50"))
				time.Sleep(800 * time.Millisecond)
				exitCh <- true
				os.Exit(0)
			}
		} else if strings.Contains("auto", lowerCommand) {
			if *modeFlag {
				fmt.Println("Currently running on auto mode ...")
			} else {
				fmt.Println("Switch to auto run mode ...")
				*modeFlag = true
				switcher <- true
			}
		} else if strings.Contains("cancel", lowerCommand) {
			if *modeFlag {
				// 从自动调速模式退出，进入阻塞状态
				fmt.Println("Exit from auto run mode ...")
				*modeFlag = false
			} else {
				fmt.Println("Running on managed mode already")
			}
		} else if _, err := strconv.ParseInt(lowerCommand, 10, 8); err == nil {
			if *modeFlag {
				fmt.Println("Currently running on auto mode ...")
			} else {
				writeCommand(serialCom, wrapCommand(lowerCommand))
			}
		} else {
			fmt.Println("Invalid command !")
		}
	}
}

func main() {
	flag.Parse()
	var reader = bufio.NewReaderSize(os.Stdin, 16)
	var conf = &serial.Config{Name: *port, Baud: 9600, ReadTimeout: 3 * time.Second, Size: 8}
	com, err := serial.OpenPort(conf)
	defer close(exitCh)
	defer close(switcher)
	defer com.Close()
	if err != nil {
		log.Fatal("[ERROR] An error occurred when open serial port !")
	}
	platformInfo()
	go managedMode(com, reader)
	go autoMode(com)
	<-exitCh
}
