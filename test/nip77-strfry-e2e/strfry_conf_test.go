//go:build e2e

package nip77strfry_test

// strfryConf is a relay config that accepts published-port connections.
// nofiles=0 avoids setrlimit failures under Docker's default ulimit.
// auth is off so this test exercises negentropy, not NIP-42.
const strfryConf = `
db = "./strfry-db/"
dbParams {
    maxreaders = 256
    mapsize = 1099511627776
    noReadAhead = false
}
events {
    maxEventSize = 65536
    rejectEventsNewerThanSeconds = 900
    rejectEventsOlderThanSeconds = 94608000
    rejectEphemeralEventsOlderThanSeconds = 60
    ephemeralEventsLifetimeSeconds = 300
    maxNumTags = 2000
    maxTagValSize = 1024
}
relay {
    bind = "0.0.0.0"
    port = 7777
    nofiles = 0
    realIpHeader = ""
    auth {
        enabled = false
        serviceUrl = ""
    }
    info {
        name = "strfry e2e"
        description = "congee nip77 e2e"
        pubkey = ""
        contact = ""
        nips = ""
    }
    maxWebsocketPayloadSize = 131072
    maxReqFilterSize = 200
    autoPingSeconds = 55
    enableTcpKeepalive = false
    queryTimesliceBudgetMicroseconds = 10000
    maxFilterLimit = 500
    maxTagsPerFilter = 3
    maxFilterLimitCount = 1000000
    maxSubsPerConnection = 20
    writePolicy {
        plugin = ""
        timeoutSeconds = 10
    }
    compression {
        enabled = false
        slidingWindow = false
    }
    logging {
        dumpInAll = false
        dumpInEvents = false
        dumpInReqs = false
        dbScanPerf = false
        invalidEvents = true
    }
    numThreads {
        ingester = 2
        reqWorker = 2
        reqMonitor = 1
        negentropy = 2
    }
    negentropy {
        enabled = true
        maxSyncEvents = 1000000
    }
}
`
