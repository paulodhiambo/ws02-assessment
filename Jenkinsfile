// Jamii Savings – one pipeline for all three integrations.
//
// Build -> unit test -> spin up dev (docker compose) -> deploy MI + APIM as one
// release -> integration/chaos tests -> (approval) -> prod.
//
// The pipeline contains no per-API logic. The lists of integrations live in
// one place each (mi/build/package_car.py, scripts/deploy-apim.sh), and every
// stage loops over all of them, so adding a fourth API needs no pipeline change.
//
// Agent requirements: Docker with the compose plugin, JDK 17+, Maven 3.9+,
// Python 3, curl, zip, rsync. apictl is installed by the pipeline.

pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()       // one deployment to the shared dev stack at a time
        timeout(time: 60, unit: 'MINUTES')
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    parameters {
        choice(name: 'BACKENDS', choices: ['mock', 'public'],
               description: 'mock: local customer/SOAP mocks (deterministic, enables failure tests). public: JSONPlaceholder + DNE Online.')
        booleanParam(name: 'PROMOTE_TO_PROD', defaultValue: false,
                     description: 'After dev passes, ask for approval and run the prod stage.')
        booleanParam(name: 'KEEP_DEV_ENV', defaultValue: false,
                     description: 'Leave the dev containers running after the build (for demos).')
    }

    environment {
        CAR_VERSION          = "1.0.${BUILD_NUMBER}"
        COMPOSE_PROJECT_NAME = 'jamii'
        APICTL_CONFIG_DIR    = "${WORKSPACE}/.apictl"
    }

    stages {
        stage('Build & package') {
            steps {
                // MI: mvn clean install -> mi/target/cars/*.car (validation runs in the validate phase)
                // APIM: each apictl project + shared policies -> dist/apim/*.zip
                sh 'scripts/build.sh --skip-tests --version "$CAR_VERSION"'
            }
        }

        stage('Unit tests') {
            steps {
                sh 'scripts/test.sh --unit'
            }
        }

        stage('Archive artifacts') {
            steps {
                // These exact files are what gets deployed to dev and promoted to prod.
                archiveArtifacts artifacts: 'mi/target/cars/*.car, dist/apim/*.zip', fingerprint: true
            }
        }

        stage('Dev: environment') {
            environment {
                CUSTOMER_BACKEND_URL  = "${params.BACKENDS == 'mock' ? 'http://customer-mock:3000' : 'https://jsonplaceholder.typicode.com'}"
                LOAN_SOAP_BACKEND_URL = "${params.BACKENDS == 'mock' ? 'http://soap-mock:8088/calculator.asmx' : 'http://www.dneonline.com/calculator.asmx'}"
            }
            steps {
                sh 'scripts/install-apictl.sh'
                sh 'docker compose --profile apim up -d --build --wait --wait-timeout 600'
            }
        }

        stage('Dev: deploy') {
            steps {
                // Both steps are all-or-nothing for their three APIs. If MI succeeds
                // and APIM then fails, MI goes back to the previous CARs too, so
                // dev never ends up with half a release.
                sh 'scripts/deploy-mi.sh dev'
                script {
                    try {
                        sh 'scripts/deploy-apim.sh dev'
                    } catch (err) {
                        echo 'APIM deployment failed - restoring the previous MI CARs'
                        sh '''
                            if ls target/mi-previous/*.car >/dev/null 2>&1; then
                              CAR_DIR=target/mi-previous scripts/deploy-mi.sh dev
                            else
                              echo "no previous MI release to restore (first deployment)"
                            fi
                        '''
                        throw err
                    }
                }
            }
        }

        stage('Dev: integration tests') {
            steps {
                sh 'scripts/test.sh --integration'          // direct to MI
                sh 'python3 scripts/apim-demo-consumer.py'  // Dev Portal onboarding: app, subscriptions, keys
                sh 'scripts/test.sh --integration --apim'   // same suite through the gateway (OAuth2 + API key)
                sh 'scripts/test.sh --chaos'                // DB / backend outages map to the right errors
            }
            post {
                always {
                    junit allowEmptyResults: true, testResults: 'target/test-reports/*.xml'
                }
            }
        }

        stage('Prod: approval') {
            when { expression { params.PROMOTE_TO_PROD } }
            steps {
                timeout(time: 1, unit: 'HOURS') {
                    input message: "Promote ${env.CAR_VERSION} to prod?", ok: 'Deploy'
                }
            }
        }

        stage('Prod: deploy') {
            when { expression { params.PROMOTE_TO_PROD } }
            steps {
                // Same archived artifacts, prod config. Not wired to a real environment
                // in this assignment: without infrastructure/config/prod.env the scripts
                // run in --dry-run mode and print the plan.
                withCredentials([
                    usernamePassword(credentialsId: 'jamii-prod-mi-admin', usernameVariable: 'MI_ADMIN_USER', passwordVariable: 'MI_ADMIN_PASSWORD'),
                    usernamePassword(credentialsId: 'jamii-prod-apim-admin', usernameVariable: 'APIM_ADMIN_USER', passwordVariable: 'APIM_ADMIN_PASSWORD')
                ]) {
                    sh '''
                        if [ -f infrastructure/config/prod.env ]; then MODE=""; else
                          cp infrastructure/config/prod.env.example infrastructure/config/prod.env; MODE="--dry-run"
                        fi
                        scripts/deploy-mi.sh prod $MODE
                        scripts/deploy-apim.sh prod $MODE
                    '''
                }
            }
        }
    }

    post {
        always {
            sh '''
                mkdir -p target/logs
                for s in mi apim customer-mock soap-mock; do docker compose logs --no-color "$s" > "target/logs/$s.log" 2>&1 || true; done
            '''
            archiveArtifacts artifacts: 'target/logs/*.log', allowEmptyArchive: true
            script {
                if (!params.KEEP_DEV_ENV) {
                    sh 'scripts/cleanup.sh --all || true'
                }
            }
        }
        failure {
            echo "Build ${env.CAR_VERSION} failed. Nothing was partially deployed: see the deploy stage log for any rollback."
        }
    }
}
