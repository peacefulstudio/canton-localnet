// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

import com.digitalasset.canton.topology.transaction.SynchronizerTrustCertificate.ParticipantTopologyFeatureFlag

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

Seq(`a-validator-1`, `b-validator-1`, `d-validator-1`).foreach { participant =>
  participant.synchronizers.list_connected().foreach { connected =>
    participant.topology.synchronizer_trust_certificates.propose(
      participant.id,
      connected.synchronizerId,
      featureFlags = Seq(ParticipantTopologyFeatureFlag.EnableMultiSynchronizer),
    )
  }
}
