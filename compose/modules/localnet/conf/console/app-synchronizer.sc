bootstrap.synchronizer(
  synchronizerName = "app-synchronizer",
  sequencers = Seq(`app-sequencer`),
  mediators = Seq(`app-mediator`),
  synchronizerOwners = Seq(`app-sequencer`),
  synchronizerThreshold = 1,
  staticSynchronizerParameters = StaticSynchronizerParameters.defaultsWithoutKMS(ProtocolVersion.latest),
)

`a-validator-1`.synchronizers.connect_local(`app-sequencer`, "app-synchronizer")
`b-validator-1`.synchronizers.connect_local(`app-sequencer`, "app-synchronizer")
`d-validator-1`.synchronizers.connect_local(`app-sequencer`, "app-synchronizer")

utils.retry_until_true {
  `a-validator-1`.synchronizers.active("app-synchronizer") &&
    `b-validator-1`.synchronizers.active("app-synchronizer") &&
    `d-validator-1`.synchronizers.active("app-synchronizer")
}
