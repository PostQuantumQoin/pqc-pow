package pqcpow

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/pqccrypto/mqphash"
)

var devslk []*sync.Mutex

const maxN = 63 //If bigger then fix it.
type controller struct {
	size      int
	fixNumber int
	fixIndex  int
	fixStr    []string

	numOfEquations int
	numOfVariables int
	devs           []*dev

	fixlk sync.Mutex
}

func NewController(mqphash *mqphash.MQPHash, nbit []byte, whichXWidth int) (*controller, error) {
	c := &controller{}
	c.numOfEquations = int(nbit[0]) + EquationsOffset
	c.numOfVariables = c.numOfEquations + 5

	c.fixIndex = 0

	c.size = int(GetDeviceCount()) // get Device number.
	fmt.Println("c.size:", c.size)
	if c.size <= 1 { // set fixnumber.
		c.fixNumber = 0
	} else if c.size <= 2 {
		c.fixNumber = 1
	} else if c.size <= 4 {
		c.fixNumber = 2
	} else if c.size <= 8 {
		c.fixNumber = 3
	} else if c.size <= 16 {
		c.fixNumber = 4
	}

	//The number of variables exceeds the number of countable variables. Need fix it.
	if maxN < c.numOfVariables {
		diffN := c.numOfVariables - maxN
		if diffN > c.fixNumber {
			c.fixNumber = diffN
		}
	}
	// fmt.Println("c.fixNumber:", c.fixNumber)

	if c.fixNumber > 0 { //create fix str Array.
		fLen := math.Pow(float64(2), float64(c.fixNumber))
		for i := 0; i < int(fLen); i++ {
			str := strconv.FormatInt(int64(i), 2)
			for j := 0; c.fixNumber > len(str); j++ {
				str = "0" + str
			}
			c.fixStr = append(c.fixStr, str)
		}
	}
	fmt.Println("c.fixStr:", c.fixStr)

	if len(devslk) != c.size { // create signals for device resources.
		for devID := 0; devID < c.size; devID++ {
			lk := new(sync.Mutex)
			devslk = append(devslk, lk)
		}
	}

	devs, err := getDevs(mqphash, nbit, whichXWidth, c) //Registered Device List.
	if err != nil {
		return nil, err
	}
	c.devs = devs
	return c, nil
}

func getDevs(mqphash *mqphash.MQPHash, nbit []byte, whichXWidth int, c *controller) ([]*dev, error) {
	var devs []*dev
	for devID := 0; devID < c.size; devID++ {
		if len(devslk) == 0 {
			return nil, fmt.Errorf("the list of device signals is empty")
		}
		dlk := devslk[devID]
		d := NewDev(mqphash, nbit, whichXWidth, c, dlk)
		devs = append(devs, d)
	}
	return devs, nil
}

func (c *controller) Run(notifs <-chan []*api.HeadChange, hgt abi.ChainEpoch) ([]byte, error) {
	//Receive blocks generated from devices
	result := make(chan []byte)
	//Notify other devices to stop mining when one of them acquires a block
	stopch := make(chan bool)
	defer close(result)
	defer close(stopch)

	//Calculate the value of x
	for devID := 0; devID < c.size; devID++ {
		go c.devs[devID].GetX(devID, 0, result, stopch)
	}

	// ticker := time.NewTicker(30 * time.Second)
	// tickerC := ticker.C
	for {
		select {
		case r := <-result:
			if len(r) == 0 {
				log.Warnf("run get x found")
				return nil, fmt.Errorf("warning get x found null")
			}
			return r, nil
			// case <-tickerC:
			// 	log.Warnf("Run out time")
			// 	return nil, fmt.Errorf("warning get x run out time")
		case n := <-notifs:
			for _, change := range n {
				if hgt < change.Val.Height() {
					log.Infow("new chain notify")
					return nil, nil
				}
				log.Infow("new chain ", "hgt:", hgt, "Height:", change.Val.Height())
			}
		}
	}
}

func (c *controller) GetNextFixStr() string {
	c.fixlk.Lock()
	defer c.fixlk.Unlock()
	if c.fixNumber == 0 ||
		len(c.fixStr) == 0 ||
		c.fixIndex >= len(c.fixStr) {
		return ""
	}

	fmt.Println("GetNextFixStr: c.fixIndex:", c.fixIndex)
	rt := c.fixStr[c.fixIndex]
	c.fixIndex++
	return rt
}

type dev struct {
	m int
	n int
	//  startSMCount: number;
	whichXWidth  int
	startSMCount int
	mqphash      *mqphash.MQPHash
	nbit         []byte
	//  child: ChildProcessWithoutNullStreams;
	deviceID   int
	controller *controller
	xbuf       []byte
	smCount    int

	lk *sync.Mutex
}

func NewDev(mqphash *mqphash.MQPHash, nbit []byte, whichXWidth int, ctr *controller, dlk *sync.Mutex) *dev {
	d := &dev{
		mqphash:     mqphash,
		nbit:        nbit,
		whichXWidth: whichXWidth,
		controller:  ctr,
		lk:          dlk,
	}
	d.m = int(nbit[0]) + EquationsOffset
	d.n = d.m + 5
	d.startSMCount = 0
	return d
}

func (d *dev) GetX(devID int, startSMCount int, results chan<- []byte, stopch chan bool) {
	if !d.lk.TryLock() {
		fmt.Println("TryLock fail dev:", devID)
		for {
			if d.lk.TryLock() {
				fmt.Println("TryLock success dev:", devID)
				break
			}
			select {
			case <-stopch:
				fmt.Println("TryLock clx chan is closed devID:", devID)
				return
			default:
			}
		}
	}
	defer d.lk.Unlock()

	d.deviceID = devID
	d.startSMCount = startSMCount
	fix := d.controller.GetNextFixStr()
	fmt.Println("GetX devID: fix: ", devID, fix)
	var verify bool = false
	for {
		if d.controller.fixNumber > 0 { //do fix
			var x []byte
			if len(fix) != 0 {
				x, _, _ = d.calculate(fix) // return d.xbuf = mf.fixBack(rx, fix)
			} else {
				results <- nil
				return
			}

			if len(x) == 0 {
				fmt.Println("x not found fix:", fix)
				fix = d.controller.GetNextFixStr()
				d.startSMCount = 0
				continue
			}
			if !d.mqphash.CheckIsSolution(x[0:d.mqphash.VariablesByte]) {
				fmt.Println(`Fix str '${fix}' check solution failed.`, fix)
				d.startSMCount = 0
				fix = d.controller.GetNextFixStr()
				continue
			}
		} else { //no fix
			_, x, _ := d.calculate(fix)
			fmt.Println("GetX calculate:", x)

			if len(x) == 0 {
				fmt.Println("Check solution failed!")
				results <- nil
				return
			}

			if !d.checkSolution(x) { //d.xbuf = xBuf
				fmt.Println("Check solution failed!")
				results <- nil
				return
			}
		}
		//Proof of generation passes validation
		if VerifyPoW(d.mqphash.Seed, d.nbit, d.xbuf) {
			verify = true
		}
		//check that the results channel is closed
		select {
		case <-stopch:
			fmt.Println("stopch chan is closed devID:", devID)
			return
		default:
			fmt.Println("check clx chan status:", devID)
		}
		if verify {
			fmt.Println("VerifyPoW is ok   d.deviceID: fix:", d.deviceID, fix)
			results <- d.xbuf
			return
		}
		d.startSMCount = d.smCount + 1
		fmt.Println("GetX d.startSMCount: d.smCount:", d.startSMCount, d.smCount)
	}

}

func (d *dev) checkSolution(solution string) bool {
	fmt.Println("checkSolution solution:", solution, len(solution))
	//
	x := solution[len(solution)-d.n : len(solution)]

	// var sf []string
	for index := 0; index < d.mqphash.UnwantedVariablesBit; index++ {
		// sf = append(sf, "0")
		x += "0"
	}
	// x = strings.Join(sf, "") + s
	fmt.Println("checkSolution x:", x, len(x))
	xBuf := make([]byte, 32)
	index := 0

	for i := 0; i < len(x); i += 8 {
		// xBuf[index] = parseInt(x.slice(i, i+8), 2)
		end := i + 8
		r, _ := strconv.ParseInt(x[i:end], 2, 32)
		// fmt.Println("checkSolution   r:%x", x[i:end], r)
		xBuf[index] = byte(r)
		index++
	}
	d.xbuf = xBuf

	fmt.Println("checkSolution xBuf:", hex.EncodeToString(xBuf), len(xBuf))
	return d.mqphash.CheckIsSolution(xBuf[0:d.mqphash.VariablesByte])
}

// private checkSolution(): boolean {
// 	let solution = this.x_data.x;
// 	let x = solution.slice(solution.length - this.n, solution.length);

// 	for (let index = 0; index < this.mqphash.MQP.unwantedVariablesBit; index++) {
// 		x += '0';
// 	}

// 	let xBuf = Buffer.alloc(32);
// 	let index = 0;

// 	for (let i = 0; i < x.length; i += 8) {
// 		xBuf[index++] = parseInt(x.slice(i, i + 8), 2);
// 	}
// 	this.x_data.xBuf = xBuf;

// 	return this.mqphash.checkIsSolution(xBuf.subarray(0, this.mqphash.MQP.variablesByte));
// }

func (d *dev) calculate(fix string) ([]byte, string, error) {
	// d.lk.Lock()
	// defer d.lk.Unlock()

	var equations []string
	var coefficientBit int

	type rxresult struct {
		X       string `json:"x"`
		GpuTime string `json:"gpuTime"`
		Rate    string `json:"rate"`
		SmCount string `json:"smCount"`
		SmUse   string `json:"smUse"`
	}

	var rs rxresult
	if len(fix) != 0 {
		mf := NewFix(d.mqphash, len(fix))
		for _, equation := range d.mqphash.Equations {
			eq, _, _, _ := mf.FixOneEquation(fix, hex.EncodeToString(equation), d.mqphash.UnwantedCoefficientBit)
			equations = append(equations, hex.EncodeToString(eq))
		}
		// fmt.Println("calculate CudaGetX fix:", fix)

		fmt.Println("calculate  d.deviceID, fix d.m, mf.newN, d.whichXWidth, uint64(d.startSMCount), mf.newCoe", d.deviceID, fix, d.m, mf.newN, d.whichXWidth, uint64(d.startSMCount), mf.newCoe)
		// for i, eq := range equations {
		// 	fmt.Printf("calculate fix:%s Equations:%s len:%d  index:%d \n", fix, eq, len(eq), i)
		// }
		rx := CudaGetX(d.deviceID, d.m, mf.newN, d.whichXWidth, uint64(d.startSMCount), mf.newCoe, equations)
		// CudaGetX(deviceID int, m int, n int, whichXWidth int, startSMCount uint64, coefficientBit int, xIn []string)
		srx := strings.Split(rx, "x found:")
		// fmt.Println("calculate CudaGetX fix: srx[1]:", fix, srx[1])
		if err := json.Unmarshal([]byte(srx[1]), &rs); err != nil {
			return nil, rx, err
		}

		num, err := strconv.Atoi(rs.SmCount)
		if err != nil {
			return nil, "", err
		}
		d.smCount = num
		fmt.Println("calculate  d.deviceID: fix:  rs.SmCount:", d.deviceID, fix, rs.SmCount)
		d.xbuf = mf.fixBack(rs.X, fix)
		// fmt.Println("calculate fix: fixBack:", fix, hex.EncodeToString(d.xbuf))
		return d.xbuf, "", nil
	} else {
		for _, equation := range d.mqphash.Equations {
			equations = append(equations, hex.EncodeToString(equation))
		}
		coefficientBit = d.mqphash.Coefficient
	}

	rx := CudaGetX(d.deviceID, d.m, d.n, d.whichXWidth, uint64(d.startSMCount), coefficientBit, equations)
	srx := strings.Split(rx, "x found:")
	if err := json.Unmarshal([]byte(srx[1]), rs); err != nil {
		return nil, rx, err
	}

	fmt.Println("calculate CudaGetX rx:", rx)
	return nil, rs.X, nil
}
