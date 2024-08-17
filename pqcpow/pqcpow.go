package pqcpow

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/pqccrypto/mqphash"
	"github.com/filecoin-project/lotus/pqccrypto/shake3"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("pqcpow")

const (
	EquationsOffset  = 31
	NbitSampleRate   = 1
	ReferenceSeconds = 15 * 1000 //15s
)

//	const powParameter = {
//		equationsOffset: 31,
//		nbitSampleRate: 200,
//		referenceSeconds: 600
//	}
type PqcPowAPI interface {
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainHead(context.Context) (*types.TipSet, error) //perm:read
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
}

/**
 * @param {Buffer} seed - Block Header, Does not contain
 * @param {Buffer} nbit - 2 byte; First is equations (Rough adjustment), last is threshold (Fine-tune)
 * @param {Buffer} x - 32byte; Hash input
 */
func VerifyPoW(seed []byte, nbit []byte, x []byte) bool {
	equationsN := int(nbit[0]) + EquationsOffset
	variablesN := equationsN + 5
	threshold := int(nbit[1])
	// fmt.Println("verifyPoW seed: nbit: x:", seed, nbit, x)
	// fmt.Println("verifyPoW equationsN: variablesN: threshold:", equationsN, variablesN, threshold)
	if len(x) != 32 {
		return false
	}

	//------- step 1 create mpqhash function -------
	powHash := mqphash.CreateMQP(seed, equationsN, variablesN)

	for i := powHash.VariablesByte; i < 32; i++ {
		if x[i] != 0 {
			return false
		}
	}

	if (x[powHash.VariablesByte-1] & (^(0xff << powHash.UnwantedVariablesBit))) != 0 {
		return false
	}

	x = x[0:powHash.VariablesByte]
	//------- step 2 mpqhash(x) = zero, Confirm whether x is the solution of the system of equations -------
	isSolution := powHash.CheckIsSolution(x)
	if !isSolution {
		return false
	}
	// fmt.Println("verifyPoW isSolution:", isSolution)
	//------- step 3 Let x2 = shake256(x), and set the extra bits to 0 -------
	x2 := shake3.Shake256XOF(x, powHash.VariablesByte)
	x2[len(x2)-1] >>= powHash.UnwantedVariablesBit
	x2[len(x2)-1] <<= powHash.UnwantedVariablesBit

	// fmt.Println("verifyPoW x2:", x2)
	//------- step 4 Get the hashvalue of mpqhash(x2) -------
	hashVal := powHash.Update(x2)
	if len(hashVal) == 0 {
		return false
	}
	// fmt.Println("verifyPoW hashVal:", hashVal)
	//------- step 5 Let hashVal = shake256(hashVal), and take the first 9 bits as the integer value -------
	hashVal2 := shake3.Shake256XOF(hashVal, 2)                // 2 byte = 16 bit
	Val2 := (int(hashVal2[0]) << 1) | (int(hashVal2[1]) >> 7) //first 9 bits to integer value (big-endian)
	// fmt.Println("verifyPoW hashVal2:", Val2)
	//------- step 6 Compare with threshold -------
	if threshold < Val2 {
		return true
	} else {
		return false
	}
}

func CalculateNbit(targetTime uint64, lastNbit []byte, windowSize uint64, windowTimeStart uint64, windowTimeEnd uint64) []byte {
	// fmt.Println("CalculateNbit targetTime: lastNbit: windowSize: windowTimeStart: windowTimeEnd:", targetTime, hex.EncodeToString(lastNbit), windowSize, windowTimeStart, windowTimeEnd)
	exponent := uint8(lastNbit[0])
	threshold := uint8(lastNbit[1])
	actualTime := (windowTimeEnd - windowTimeStart) * 1000 / windowSize
	timeRatio := float64(actualTime) / float64(targetTime)
	speedRatio := 1 / timeRatio
	fmt.Println("CalculateNbit actualTime: timeRatio: speedRatio:", actualTime, timeRatio, speedRatio)
	nbitDigitize := float64(exponent) + math.Log2(512/(512-float64(threshold)))
	newNbitDigitize := nbitDigitize + math.Log2(float64(speedRatio))
	var newExponent, newThreshold float64
	// fmt.Println("CalculateNbit nbitDigitize: newNbitDigitize: ", nbitDigitize, newNbitDigitize)
	if newNbitDigitize < 1 {
		newNbitDigitize = 1
		newExponent = 1
		newThreshold = 0
	} else {
		newExponent = math.Floor(newNbitDigitize)
		newThreshold = 512 / math.Pow(2, (newNbitDigitize-newExponent))
		newThreshold = math.Floor(512 - newThreshold)
	}

	if newThreshold > 255 {
		newThreshold = 255
	} else if newThreshold < 0 {
		newThreshold = 0
	}
	fmt.Println("CalculateNbit newExponent: newThreshold: ", newExponent, newThreshold)
	newNbit := make([]byte, 2)
	newNbit[0] = uint8(newExponent)
	newNbit[1] = uint8(newThreshold)

	return newNbit
}

func GetNbit(ctx context.Context, p PqcPowAPI) ([]byte, error) {
	newNbit := make([]byte, 2)
	lastNbit := make([]byte, 2) //lastTipSet.Nbit()

	lastTipSet, err := p.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	lastNbit = lastTipSet.PqcPowProof().Nbit
	if lastTipSet.Height() != 0 && lastTipSet.Height()%NbitSampleRate == 0 {
		sampleTipSet, err := p.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(lastTipSet.Height()-NbitSampleRate), types.EmptyTSK)
		if err != nil {
			return nil, err
		}
		startTime := sampleTipSet.MinTimestamp() + build.PropagationDelaySecs
		endTime := lastTipSet.MinMiningTimestamp()

		log.Infow("GetNbit ", "lastNbit:", lastNbit, "lastTipSet.Height: ",
			lastTipSet.Height(), "startTime:", time.Unix(int64(startTime), 0),
			"endTime:", time.Unix(int64(endTime), 0))
		newNbit = CalculateNbit(ReferenceSeconds, lastNbit, NbitSampleRate, startTime, endTime)
	} else {
		log.Infow("GetNbit ", "lastNbit:%x", lastNbit, "lastTipSet.Height: ", lastTipSet.Height())
		newNbit = lastNbit
	}
	return newNbit, nil
}

func PqcPowProof(ctx context.Context, seed []byte, nbit []byte, p PqcPowAPI) ([]byte, error) {
	m := int(nbit[0]) + EquationsOffset
	n := m + 5
	mh := mqphash.CreateMQP(seed, m, n)

	var whichXWidth int = 1000
	c, err := NewController(mh, nbit, whichXWidth)
	if err != nil {
		return nil, err
	}
	ts, err := p.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	notifs, err := p.ChainNotify(ctx)
	if err != nil {
		return nil, err
	}
	x, err := c.Run(notifs, ts.Height())
	if err != nil {
		return nil, err
	}
	return x, nil
}

func getDifficultyByNbit(nbit []byte) float64 {
	exponent := float64(nbit[0])
	threshold := float64(nbit[1])
	return exponent + math.Log2(512/(512-threshold))
}
