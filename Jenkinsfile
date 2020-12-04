@Library('dst-shared@release/shasta-1.4') _

dockerBuildPipeline {
        repository = "cray"
        imagePrefix = "cray"
        app = "hmnfd"
        name = "hms-hmnfd"
        description = "Cray State Change Notification Fanout Service"
        dockerfile = "Dockerfile"
        unitTestScript = "runUnitTest.sh"
        slackNotification = ["", "", false, false, true, true]
        product = "csm"
}
