# rubocop:disable all

class ListenerMode

  HUB_ENDPOINT = "http://localhost:4567"

  def boot_up_agent
    system "docker stop $(docker ps -q)"
    system "docker rm $(docker ps -qa)"
    system "docker-compose -f test/e2e_support/docker-compose-listen.yml build"
    system "docker-compose -f test/e2e_support/docker-compose-listen.yml up -d"

    wait_for_agent_to_register_in_the_hub
  end

  def start_job(request)
    false
  end

  def wait_for_command_to_start(cmd)
    sleep 3
    false
  end

  def wait_for_job_to_finish
    sleep 10
    false
  end

  private

  def wait_for_agent_to_register_in_the_hub
    loop do
      puts "Waiting for agent to register in the hub"

      response = `curl --fail -X GET -k "#{HUB_ENDPOINT}/private/is_registered"`

      if response == "yes"
        break
      else
        sleep 1
      end
    end
  end

end
