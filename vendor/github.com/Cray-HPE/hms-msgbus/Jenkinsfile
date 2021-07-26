@Library('dst-shared@master') _

dockerBuildPipeline {
        githubPushRepo = "Cray-HPE/hms-msgbus"
        repository = "cray"
        imagePrefix = "hms"
        app = "msgbus"
        name = "hms-msgbus"
        description = "Cray HMS msgbus code."
        dockerfile = "Dockerfile"
        slackNotification = ["", "", false, false, true, true]
        product = "internal"
}
