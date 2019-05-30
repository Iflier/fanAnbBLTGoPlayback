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

func writeCommand(serialCom *serial.Port, command string) error {
	writtenNum, err := serialCom.Write([]byte(command))
	if err != nil {
		fmt.Printf("[ERROR] An error occurred when write coammand to device: %s", err)
		return err
	}
	fmt.Printf("[INFO] Written %d bytes\n", writtenNum)
	return nil
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
			fmt.Println("[ERROR] An error occurred when read string from console !")
			break
		}
		fmt.Printf("Received command: %s", command)
		lowerCommand := strings.ToLower(strings.Trim(command, "\r\n.( )"))
		if lowerCommand != "" {
			if strings.Contains("exitquit", lowerCommand) {
				if *modeFlag {
					fmt.Println("Currently running on auto mode, please use \"cancel\" command switch to managed mode firstly")
				} else {
					err := writeCommand(serialCom, wrapCommand("50"))
					if err != nil {
						break
					}
					time.Sleep(500 * time.Millisecond)
					exitCh <- true
					time.Sleep(100 * time.Millisecond) // 2019.05.25 方便main退出。如果不加，该函数还会再run一轮
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
			} else if digitValue, err := strconv.ParseInt(lowerCommand, 10, 8); err == nil {
				if *modeFlag {
					fmt.Println("Currently running on auto mode ...")
				} else {
					if 0 <= digitValue && digitValue <= 100 {
						// 尽管在把字符串解析为数值的时候，对可能的数值做了限制，但是仍有接收到101 ~ 127的可能
						err := writeCommand(serialCom, wrapCommand(lowerCommand))
						if err != nil {
							break
						}
					} else {
						fmt.Printf("[WARNING] Valid input in range: 0 ~ 100, received: %s\n", lowerCommand)
					}
				}
			} else {
				fmt.Println("Invalid command !")
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
	// defer fmt.Println("Happy") // for test
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
