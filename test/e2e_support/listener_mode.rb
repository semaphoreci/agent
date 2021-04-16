class ListenerMode

  def boot_up_agent
    system "docker stop $(docker ps -q)"
    system "docker rm $(docker ps -qa)"
    system "docker-compose -f test/e2e_support/docker-compose-listen.yml build"
    system "docker-compose -f test/e2e_support/docker-compose-listen.yml up -d"
  end

end
