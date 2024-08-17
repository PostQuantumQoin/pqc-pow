package main

import "C"
import (
	"fmt"

	"pqcpow"
)

// func GetDeviceCount() C.int {
// 	return C.cudaGetDevCount()
// }

// func GetNumOfExecution(n int32, m int32) C.ulonglong {
// 	return C.cudaGetNumOfExecution(C.int(n), C.int(m))
// }

// func CudaGetX(deviceID int, m int, n int, whichXWidth int, startSMCount uint64, coefficientBit int, xIn [][]byte) string {
// 	x := make([]*C.char, 0)

// 	for index, value := range xIn {
// 		x[index] = (*C.char)(C.CBytes(value))
// 	}
// 	xstr := C.cudaGetX(C.int(deviceID), C.int(m), C.int(n), C.int(whichXWidth),
// 		C.ulonglong(startSMCount), C.int(coefficientBit), &x[0])

// 	gxstr := C.GoString(xstr)

// 	C.free(unsafe.Pointer(xstr)) //xstr inside cuda malloc, golang external free xstr

// 	for a := 0; a < 16; a++ {
// 		C.free(unsafe.Pointer(x[a])) //free x[]*C.char
// 	}

// 	return gxstr
// }

func main() {

	fmt.Println("cuda test start!")
	// CudaPrint()

	var deviceID, m, n, whichXWidth, coefficientBit int
	var startSMCount uint64
	// 2 0 16 21 1000 0 232
	deviceID = 0
	m = 16
	n = 21
	whichXWidth = 1000
	coefficientBit = 232
	startSMCount = 0

	x := make([]string, 16)
	x[0] = "c33ec945e540598926767d47c3600cd65abb833075ffc2d27a0f7d0089"
	x[1] = "8be0fd93604aed214c05b8d18eff99fba72198a498368c45651af6fdc6"
	x[2] = "f979fef175fd6385117ec5390518322dfd89fb5bf4367bff95dde87d51"
	x[3] = "117c8d533b878fffad347af8ea40d33b049f282961f313b2e44641833f"
	x[4] = "117c8d533b878fffad347af8ea40d33b049f282961f313b2e44641833f"
	x[5] = "93f34be77666df64d0ade2a2c6b3786c916cada0091ae7184de65b9ddf"
	x[6] = "f587f5fdfbd84fbdd7269c44d9a8ac55c5ee97d01477aa3e40f68c9c7e"
	x[7] = "362277213c1e23112fe70a7d06803e1bfcca5d832197c5a8ff53b4d374"
	x[8] = "080ab9e431ecaac2c42b72a50750e4075ad24bcbfe529842bf01584561"
	x[9] = "ed3ef7679d0435c9bba00d6989b856b016b5edfb501164030898471133"
	x[10] = "3c77f20060e8f8b3c95f95e141bda3a5ee98e8f34a286a0e9dfd93c567"
	x[11] = "4df5ed21be4a70771d05326fdef1ab45f367a6fd3980cd614949bc6d5e"
	x[12] = "5e6422418934982ad8171adfc6b1ef8fd503398b004f7a97da2a09e0ba"
	x[13] = "80e9a5262d5b26400cdeee8872b49a0333a1821914d9955a71bfd3aacf"
	x[14] = "511782411cab7fb1f57e012944e0279f11d895a22250b2443e49d1d2aa"
	x[15] = "d209cad023a0fdb92d4795e537bc8887c32cf5fa3805cf3be168b52251"

	gxstr := pqcpow.CudaGetX(deviceID, m, n, whichXWidth, startSMCount, coefficientBit, x)
	fmt.Printf("pqcpow cuda CudaGetX:%s\n", gxstr) // 支持格式化输出

	// var devcunt = GetDeviceCount()
	// fmt.Println("devcunt:%d",devcunt)

	// var NumOfE = GetNumOfExecution(21, 16)
	// fmt.Println("NumOfE:%d",NumOfE)

}
