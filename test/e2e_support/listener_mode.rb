class ListenerMode

  def boot_up_agent
    system "docker stop $(docker ps -q)"
    system "docker rm $(docker ps -qa)"
    system "docker build -t agent -f Dockerfile.test ."
    system "docker run --privileged --device /dev/ptmx -v /tmp/agent-temp-directory/:/tmp/agent-temp-directory -v /var/run/docker.sock:/var/run/docker.sock -p #{$AGENT_PORT_IN_TESTS}:8000 -p #{$AGENT_SSH_PORT_IN_TESTS}:22 --name agent -tdi agent bash -c \"service ssh restart && ./agent start\""
  end

end
