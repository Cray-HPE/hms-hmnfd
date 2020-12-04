@Library('dst-shared@master') _

dockerBuildPipeline {
        repository = "cray"
        imagePrefix = "hms"
        app = "msgbus"
        name = "hms-msgbus"
        description = "Cray HMS msgbus code."
        dockerfile = "Dockerfile"
        slackNotification = ["", "", false, false, true, true]
        product = "internal"
}
