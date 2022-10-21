package main

import (
	"eth2-exporter/rpc"
	"eth2-exporter/types"
	"eth2-exporter/utils"
	"eth2-exporter/version"
	"flag"
	"math/big"
	"time"

	geth_rpc "github.com/ethereum/go-ethereum/rpc"

	eth_rewards "github.com/gobitfly/eth-rewards"
	"github.com/prysmaticlabs/prysm/v3/api/client/beacon"
	"github.com/sirupsen/logrus"
)

func main() {

	configPath := flag.String("config", "", "Path to the config file, if empty string defaults will be used")
	bnAddress := flag.String("beacon-node-address", "", "Url of the beacon node api")
	enAddress := flag.String("execution-node-address", "", "Url of the execution node api")

	flag.Parse()

	cfg := &types.Config{}
	err := utils.ReadConfig(cfg, *configPath)
	if err != nil {
		logrus.Fatalf("error reading config file: %v", err)
	}
	utils.Config = cfg
	logrus.WithField("config", *configPath).WithField("version", version.Version).WithField("chainName", utils.Config.Chain.Config.ConfigName).Printf("starting")

	client, err := beacon.NewClient(*bnAddress)
	if err != nil {
		logrus.Fatal(err)
	}

	lc, err := rpc.NewLighthouseClient(*bnAddress, big.NewInt(5))
	if err != nil {
		logrus.Fatal(err)
	}

	head, err := lc.GetChainHead()
	if err != nil {
		logrus.Fatal(err)
	}

	elClient, err := geth_rpc.Dial(*enAddress)
	if err != nil {
		logrus.Fatal(err)
	}
	start := time.Now()
	logrus.Infof("retrieving rewards details for epoch %v", head.FinalizedEpoch)

	rewards, err := eth_rewards.GetRewardsForEpoch(int(head.FinalizedEpoch), client, elClient)

	if err != nil {
		logrus.Errorf("error retrieving reward details for epoch %v", head.FinalizedEpoch)
	} else {
		logrus.Infof("retrieved %v reward details for epoch %v in %v", len(rewards), head.FinalizedEpoch, time.Since(start))
	}
}
