/*
 * Copyright 2020 ICON Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package iiss

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/icon-project/goloop/common"
	"github.com/icon-project/goloop/common/db"
	"github.com/icon-project/goloop/common/errors"
	"github.com/icon-project/goloop/common/log"
	"github.com/icon-project/goloop/icon/icmodule"
	"github.com/icon-project/goloop/icon/iiss/icreward"
	"github.com/icon-project/goloop/icon/iiss/icstage"
	"github.com/icon-project/goloop/icon/iiss/icstate"
)

func MakeCalculator(database db.Database, back *icstage.Snapshot) *Calculator {
	c := new(Calculator)
	c.back = back
	c.base = icreward.NewSnapshot(database, nil)
	c.temp = c.base.NewState()
	c.log = log.New()

	return c
}

func TestCalculator_processClaim(t *testing.T) {
	database := db.NewMapDB()
	front := icstage.NewState(database)

	addr1 := common.MustNewAddressFromString("hx1")
	addr2 := common.MustNewAddressFromString("hx2")
	v1 := int64(100)
	v2 := int64(200)

	type args struct {
		addr  *common.Address
		value *big.Int
	}

	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			"Add Claim 100",
			args{
				addr1,
				big.NewInt(v1),
			},
			v1,
		},
		{
			"Add Claim 200",
			args{
				addr2,
				big.NewInt(v2),
			},
			v2,
		},
	}

	// initialize data
	c := MakeCalculator(database, nil)
	for _, tt := range tests {
		args := tt.args
		// temp IScore : args.value * 2
		iScore := icreward.NewIScore(new(big.Int).Mul(args.value, big.NewInt(2)))
		err := c.temp.SetIScore(args.addr, iScore)
		assert.NoError(t, err)

		// add Claim : args.value
		_, err = front.AddIScoreClaim(args.addr, args.value)
		assert.NoError(t, err)
	}
	c.back = front.GetSnapshot()

	err := c.processClaim()
	assert.NoError(t, err)

	// check result
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			iScore, err := c.temp.GetIScore(args.addr)
			assert.NoError(t, err)
			assert.Equal(t, 0, args.value.Cmp(iScore.Value()))
		})
	}
}

func TestCalculator_processBlockProduce(t *testing.T) {
	addr0 := common.MustNewAddressFromString("hx0")
	addr1 := common.MustNewAddressFromString("hx1")
	addr2 := common.MustNewAddressFromString("hx2")
	addr3 := common.MustNewAddressFromString("hx3")
	variable := big.NewInt(int64(YearBlock * icmodule.IScoreICXRatio))
	rewardGenerate := variable.Int64()
	rewardValidate := variable.Int64()

	type args struct {
		bp       *icstage.BlockProduce
		variable *big.Int
	}
	tests := []struct {
		name  string
		args  args
		err   bool
		wants [4]int64
	}{
		{
			name: "Zero Irep",
			args: args{
				icstage.NewBlockProduce(0, 0, new(big.Int).SetInt64(int64(0b0))),
				new(big.Int),
			},
			err:   false,
			wants: [4]int64{0, 0, 0, 0},
		},
		{
			name: "All voted",
			args: args{
				icstage.NewBlockProduce(0, 4, new(big.Int).SetInt64(int64(0b1111))),
				variable,
			},
			err: false,
			wants: [4]int64{
				rewardGenerate,
				rewardValidate / 3,
				rewardValidate / 3,
				rewardValidate / 3,
			},
		},
		{
			name: "3 P-Rep voted include proposer",
			args: args{
				icstage.NewBlockProduce(2, 3, new(big.Int).SetInt64(int64(0b0111))),
				variable,
			},
			err: false,
			wants: [4]int64{
				rewardValidate / 2,
				rewardValidate / 2,
				rewardGenerate,
				0,
			},
		},
		{
			name: "3 P-Rep voted exclude proposer",
			args: args{
				icstage.NewBlockProduce(2, 3, new(big.Int).SetInt64(int64(0b1011))),
				variable,
			},
			err: false,
			wants: [4]int64{
				rewardValidate / 3,
				rewardValidate / 3,
				rewardGenerate,
				rewardValidate / 3,
			},
		},
		{
			name: "Invalid proposerIndex",
			args: args{
				icstage.NewBlockProduce(5, 3, new(big.Int).SetInt64(int64(0b0111))),
				variable,
			},
			err:   true,
			wants: [4]int64{0, 0, 0, 0},
		},
		{
			name: "There is no validator Info. for voter",
			args: args{
				icstage.NewBlockProduce(5, 16, new(big.Int).SetInt64(int64(0b01111111111111111))),
				variable,
			},
			err:   true,
			wants: [4]int64{0, 0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.args
			vs := makeVS(addr0, addr1, addr2, addr3)
			err := processBlockProduce(in.bp, in.variable, vs)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for i, v := range vs {
					assert.Equal(t, tt.wants[i], v.IScore().Int64(), "index %d", i)
				}
			}
		})
	}
}

func makeVS(addrs ...*common.Address) []*validator {
	vs := make([]*validator, 0)

	for _, addr := range addrs {
		vs = append(vs, newValidator(addr))
	}
	return vs
}

func TestCalculator_varForVotedReward(t *testing.T) {
	tests := []struct {
		name                string
		args                icstage.Global
		multiplier, divider int64
	}{
		{
			"Global Version1",
			icstage.NewGlobalV1(
				icstate.IISSVersion2,
				0,
				100-1,
				icmodule.RevisionIISS,
				big.NewInt(YearBlock),
				big.NewInt(200),
				22,
				100,
			),
			//	multiplier = ((irep * MonthPerYear) / (YearBlock * 2)) * 100 * IScoreICXRatio
			((YearBlock * MonthPerYear) / (YearBlock * 2)) * 100 * icmodule.IScoreICXRatio,
			1,
		},
		{
			"Global Version1 - disabled",
			icstage.NewGlobalV1(
				icstate.IISSVersion2,
				0,
				100-1,
				icmodule.RevisionIISS,
				big.NewInt(0),
				big.NewInt(200),
				22,
				100,
			),
			0,
			1,
		},
		{
			"Global Version2",
			icstage.NewGlobalV2(
				icstate.IISSVersion3,
				0,
				1000-1,
				icmodule.RevisionEnableIISS3,
				big.NewInt(10000),
				big.NewInt(50),
				big.NewInt(50),
				big.NewInt(0),
				big.NewInt(0),
				100,
				5,
			),
			// 	variable = iglobal * iprep * IScoreICXRatio / (100 * TermPeriod)
			10000 * 50 * icmodule.IScoreICXRatio,
			100 * MonthBlock,
		},
		{
			"Global Version2 - disabled",
			icstage.NewGlobalV2(
				icstate.IISSVersion3,
				0,
				-1,
				icmodule.RevisionEnableIISS3,
				big.NewInt(0),
				big.NewInt(0),
				big.NewInt(0),
				big.NewInt(0),
				big.NewInt(0),
				0,
				0,
			),
			0,
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multiplier, divider := varForVotedReward(tt.args)
			assert.Equal(t, tt.multiplier, multiplier.Int64())
			assert.Equal(t, tt.divider, divider.Int64())
		})
	}
}

func newVotedDataForTest(enable bool, delegated int64, bonded int64, bondRequirement int, iScore int64) *votedData {
	voted := icreward.NewVoted()
	voted.SetEnable(enable)
	voted.SetDelegated(big.NewInt(delegated))
	voted.SetBonded(big.NewInt(bonded))
	voted.SetBondedDelegation(big.NewInt(0))
	data := newVotedData(voted)
	data.SetIScore(big.NewInt(iScore))
	data.UpdateBondedDelegation(bondRequirement)
	return data
}

func TestDelegatedData_compare(t *testing.T) {
	d1 := newVotedDataForTest(true, 10, 0, 0, 10)
	d2 := newVotedDataForTest(true, 20, 0, 0, 20)
	d3 := newVotedDataForTest(true, 20, 0, 0, 21)
	d4 := newVotedDataForTest(false, 30, 0, 0, 30)
	d5 := newVotedDataForTest(false, 31, 0, 0, 31)
	type args struct {
		d1 *votedData
		d2 *votedData
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			"x<y",
			args{d1, d2},
			-1,
		},
		{
			"x<y,disable",
			args{d5, d2},
			-1,
		},
		{
			"x==y",
			args{d2, d3},
			0,
		},
		{
			"x==y,disable",
			args{d4, d5},
			0,
		},
		{
			"x>y",
			args{d3, d1},
			1,
		},
		{
			"x>y,disable",
			args{d1, d4},
			1,
		},
	}
	for _, tt := range tests {
		args := tt.args
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, args.d1.Compare(args.d2))
		})
	}
}

func TestVotedInfo_setEnable(t *testing.T) {
	totalVoted := new(big.Int)
	vInfo := newVotedInfo(100)
	status := icstage.ESDisablePermanent
	for i := int64(1); i < 6; i += 1 {
		status = status % icstage.ESMax
		addr := common.MustNewAddressFromString(fmt.Sprintf("hx%d", i))
		data := newVotedDataForTest(status.IsEnabled(), i, i, 1, 0)
		vInfo.AddVotedData(addr, data)
		if status.IsEnabled() {
			totalVoted.Add(totalVoted, data.GetVotedAmount())
		}
	}
	assert.Equal(t, 0, totalVoted.Cmp(vInfo.TotalVoted()))

	status = icstage.ESEnable
	for key, vData := range vInfo.PReps() {
		status = status % icstage.ESMax
		addr, err := common.NewAddress([]byte(key))
		assert.NoError(t, err)

		if status.IsEnabled() != vData.Enable() {
			if status.IsEnabled() {
				totalVoted.Add(totalVoted, vData.GetVotedAmount())
			} else {
				totalVoted.Sub(totalVoted, vData.GetVotedAmount())
			}
		}
		vInfo.SetEnable(addr, status)
		assert.Equal(t, status, vData.Status())
		assert.Equal(t, status.IsEnabled(), vData.Enable())
		assert.Equal(t, 0, totalVoted.Cmp(vInfo.TotalVoted()))
	}

	addr := common.MustNewAddressFromString("hx123412341234")
	vInfo.SetEnable(addr, icstage.ESDisablePermanent)
	prep := vInfo.GetPRepByAddress(addr)
	assert.Equal(t, false, prep.Enable())
	assert.True(t, prep.IsEmpty())
	assert.Equal(t, 0, prep.IScore().Sign())
	assert.Equal(t, 0, totalVoted.Cmp(vInfo.TotalVoted()))
}

func TestVotedInfo_updateDelegated(t *testing.T) {
	vInfo := newVotedInfo(100)
	votes := make([]*icstage.Vote, 0)
	enable := true
	for i := int64(1); i < 6; i += 1 {
		enable = !enable
		addr := common.MustNewAddressFromString(fmt.Sprintf("hx%d", i))
		data := newVotedDataForTest(enable, i, i, 1, 0)
		vInfo.AddVotedData(addr, data)

		votes = append(votes, icstage.NewVote(addr, big.NewInt(i)))
	}
	newAddr := common.MustNewAddressFromString("hx321321")
	votes = append(votes, icstage.NewVote(newAddr, big.NewInt(100)))

	totalVoted := new(big.Int).Set(vInfo.TotalVoted())
	vInfo.UpdateDelegated(votes)
	for _, v := range votes {
		expect := v.Amount().Int64() * 2
		if v.To().Equal(newAddr) {
			expect = v.Amount().Int64()
		}
		vData := vInfo.GetPRepByAddress(v.To())
		assert.Equal(t, expect, vData.GetDelegated().Int64())

		if vData.Enable() {
			totalVoted.Add(totalVoted, v.Amount())
		}
	}
	assert.Equal(t, 0, totalVoted.Cmp(vInfo.TotalVoted()))
}

func TestVotedInfo_updateBonded(t *testing.T) {
	vInfo := newVotedInfo(100)
	votes := make([]*icstage.Vote, 0)
	enable := true
	for i := int64(1); i < 6; i += 1 {
		enable = !enable
		addr := common.MustNewAddressFromString(fmt.Sprintf("hx%d", i))
		data := newVotedDataForTest(enable, i, i, 1, 0)
		vInfo.AddVotedData(addr, data)

		votes = append(votes, icstage.NewVote(addr, big.NewInt(i)))
	}
	newAddr := common.MustNewAddressFromString("hx321321")
	votes = append(votes, icstage.NewVote(newAddr, big.NewInt(100)))

	totalVoted := new(big.Int).Set(vInfo.TotalVoted())
	vInfo.UpdateBonded(votes)
	for _, v := range votes {
		expect := v.Amount().Int64() * 2
		if v.To().Equal(newAddr) {
			expect = v.Amount().Int64()
		}
		vData := vInfo.GetPRepByAddress(v.To())
		assert.Equal(t, expect, vData.GetBonded().Int64())

		if vData.Enable() {
			totalVoted.Add(totalVoted, v.Amount())
		}
	}
	assert.Equal(t, 0, totalVoted.Cmp(vInfo.TotalVoted()))
}

func TestVotedInfo_SortAndUpdateTotalBondedDelegation(t *testing.T) {
	d := newVotedInfo(100)
	total := int64(0)
	more := int64(10)
	maxIndex := int64(d.MaxRankForReward()) + more
	for i := int64(1); i <= maxIndex; i += 1 {
		addr := common.MustNewAddressFromString(fmt.Sprintf("hx%d", i))
		data := newVotedDataForTest(true, i, 0, 0, i)
		d.AddVotedData(addr, data)
		if i > more {
			total += i
		}
	}
	d.Sort()
	d.UpdateTotalBondedDelegation()
	assert.Equal(t, total, d.TotalBondedDelegation().Int64())

	for i, rank := range d.Rank() {
		addr := common.MustNewAddressFromString(fmt.Sprintf("hx%d", maxIndex-int64(i)))
		assert.Equal(t, string(addr.Bytes()), rank)
	}
}

func TestVotedInfo_calculateReward(t *testing.T) {
	vInfo := newVotedInfo(100)
	total := int64(0)
	more := int64(10)
	maxIndex := int64(vInfo.MaxRankForReward()) + more
	for i := int64(1); i <= maxIndex; i += 1 {
		addr := common.MustNewAddressFromString(fmt.Sprintf("hx%d", i))
		data := newVotedDataForTest(true, i, 0, 0, 0)
		vInfo.AddVotedData(addr, data)
		if i > more {
			total += i
		}
	}
	vInfo.Sort()
	vInfo.UpdateTotalBondedDelegation()
	assert.Equal(t, total, vInfo.TotalBondedDelegation().Int64())

	variable := big.NewInt(YearBlock)
	divider := big.NewInt(1)
	period := 10000
	bigIntPeriod := big.NewInt(int64(period))

	vInfo.CalculateReward(variable, divider, period)

	for i, addrKey := range vInfo.Rank() {
		expect := big.NewInt(maxIndex - int64(i))
		if i >= vInfo.MaxRankForReward() {
			expect.SetInt64(0)
		} else {
			expect.Mul(expect, variable)
			expect.Mul(expect, bigIntPeriod)
			expect.Div(expect, vInfo.TotalBondedDelegation())
		}
		assert.Equal(t, expect.Int64(), vInfo.PReps()[addrKey].IScore().Int64())
	}
}

func TestCalculator_varForVotingReward(t *testing.T) {
	type args struct {
		global            icstage.Global
		totalVotingAmount *big.Int
	}
	type want struct {
		multiplier int64
		divider    int64
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			"Global Version1",
			args{
				icstage.NewGlobalV1(
					icstate.IISSVersion2,
					0,
					100-1,
					icmodule.RevisionIISS,
					big.NewInt(MonthBlock),
					big.NewInt(20000000),
					22,
					100,
				),
				nil,
			},
			want{
				RrepMultiplier * 20000000 * icmodule.IScoreICXRatio,
				YearBlock * RrepDivider,
			},
		},
		{
			"Global Version1 - disabled",
			args{
				icstage.NewGlobalV1(
					icstate.IISSVersion2,
					0,
					100-1,
					icmodule.RevisionIISS,
					big.NewInt(MonthBlock),
					big.NewInt(0),
					22,
					100,
				),
				nil,
			},
			want{
				0,
				0,
			},
		},
		{
			"Global Version2",
			args{
				icstage.NewGlobalV2(
					icstate.IISSVersion3,
					0,
					1000-1,
					icmodule.RevisionEnableIISS3,
					big.NewInt(10000),
					big.NewInt(50),
					big.NewInt(50),
					big.NewInt(0),
					big.NewInt(0),
					100,
					5,
				),
				big.NewInt(10),
			},
			// 	multiplier = iglobal * ivoter * IScoreICXRatio / (100 * TermPeriod, totalVotingAmount)
			want{
				10000 * 50 * icmodule.IScoreICXRatio,
				100 * MonthBlock * 10,
			},
		},
		{
			"Global Version2 - disabled",
			args{
				icstage.NewGlobalV2(
					icstate.IISSVersion3,
					0,
					0-1,
					icmodule.RevisionIISS,
					big.NewInt(0),
					big.NewInt(0),
					big.NewInt(0),
					big.NewInt(0),
					big.NewInt(0),
					0,
					0,
				),
				big.NewInt(10),
			},
			want{
				0,
				0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multiplier, divider := varForVotingReward(tt.args.global, tt.args.totalVotingAmount)
			assert.Equal(t, tt.want.multiplier, multiplier.Int64())
			assert.Equal(t, tt.want.divider, divider.Int64())
		})
	}
}

type testGlobal struct {
	icstage.Global
	iissVersion int
}

func (tg *testGlobal) GetIISSVersion() int {
	return tg.iissVersion
}

func TestCalculator_VotingReward(t *testing.T) {
	addr1 := common.MustNewAddressFromString("hx1")
	addr2 := common.MustNewAddressFromString("hx2")
	addr3 := common.MustNewAddressFromString("hx3")
	addr4 := common.MustNewAddressFromString("hx4")
	prepInfo := map[string]*pRepEnable{
		string(addr1.Bytes()): {0, 0},
		string(addr2.Bytes()): {10, 0},
		string(addr3.Bytes()): {100, 200},
	}

	d0 := icstate.NewDelegation(addr1, big.NewInt(MinDelegation-1))
	d1 := icstate.NewDelegation(addr1, big.NewInt(MinDelegation))
	d2 := icstate.NewDelegation(addr2, big.NewInt(MinDelegation))
	d3 := icstate.NewDelegation(addr3, big.NewInt(MinDelegation))
	d4 := icstate.NewDelegation(addr4, big.NewInt(MinDelegation))
	type args struct {
		iissVersion int
		multiplier  int
		divider     int
		from        int
		to          int
		delegating  icstate.Delegations
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "Delegate too small in IISS 2.x",
			args: args{
				icstate.IISSVersion2,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d0},
			},
			want: 0,
		},
		{
			name: "Delegate too small in IISS 3.x",
			args: args{
				icstate.IISSVersion3,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d0},
			},
			want: 100 * d0.Value.Int64() * 1000 / 10,
		},
		{
			name: "PRep-full",
			args: args{
				icstate.IISSVersion3,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d1},
			},
			want: 100 * d1.Value.Int64() * 1000 / 10,
		},
		{
			name: "PRep-enabled",
			args: args{
				icstate.IISSVersion3,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d2},
			},
			want: 100 * d2.Value.Int64() * (1000 - 10) / 10,
		},
		{
			name: "PRep-disabled",
			args: args{
				icstate.IISSVersion3,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d3},
			},
			want: 100 * d3.Value.Int64() * (200 - 100) / 10,
		},
		{
			name: "PRep-None",
			args: args{
				icstate.IISSVersion3,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d4},
			},
			want: 0,
		},
		{
			name: "PRep-combination",
			args: args{
				icstate.IISSVersion3,
				100,
				10,
				0,
				1000,
				icstate.Delegations{d1, d2, d3, d4},
			},
			want: (100*d1.Value.Int64()*1000)/10 +
				(100*d2.Value.Int64()*(1000-10))/10 +
				(100*d3.Value.Int64()*(200-100))/10,
		},
	}

	calculator := new(Calculator)
	calculator.log = log.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			calculator.global = &testGlobal{iissVersion: args.iissVersion}
			reward := calculator.votingReward(
				big.NewInt(int64(args.multiplier)),
				big.NewInt(int64(args.divider)),
				args.from,
				args.to,
				prepInfo,
				args.delegating.Iterator(),
			)
			assert.Equal(t, tt.want, reward.Int64())
		})
	}
}

func TestCalculator_WaitResult(t *testing.T) {
	c := &Calculator{
		startHeight: InitBlockHeight,
	}
	err := c.WaitResult(1234)
	assert.NoError(t, err)

	c = &Calculator{
		startHeight: 3414,
	}
	err = c.WaitResult(1234)
	assert.Error(t, err)

	toTC := make(chan string, 2)

	go func() {
		err := c.WaitResult(3414)
		assert.True(t, err == errors.ErrInvalidState)
		toTC <- "done"
	}()
	time.Sleep(time.Millisecond*10)

	c.setResult(nil, errors.ErrInvalidState)
	assert.Equal(t, "done", <-toTC)


	c = &Calculator{
		startHeight: 3414,
	}
	go func() {
		err := c.WaitResult(3414)
		assert.NoError(t, err)
		toTC <- "done"
	}()
	go func() {
		err := c.WaitResult(3414)
		assert.NoError(t, err)
		toTC <- "done"
	}()
	time.Sleep(time.Millisecond*20)

	mdb := db.NewMapDB()
	rss := icreward.NewSnapshot(mdb, nil)
	c.setResult(rss, nil)

	assert.Equal(t, "done", <-toTC)
	assert.Equal(t, "done", <-toTC)

	assert.True(t, c.Result() == rss)
}