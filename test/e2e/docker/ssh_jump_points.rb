#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

public_key = Base64.strict_encode64("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7rLpruahEKoe2laMMnte8hHBqHKH7z9x9ZeUU78ogOCtAp0jytbMCUoB99j7P+7CsIrkjLHpCb8UnlXNwT7rl1sit0ntt01W0lVintGrNIEjSCl3eGZkPQGVP+/O55SByVOfkXtp+YwgEg2Bx1zjRHLu9zPVkvqyaC/afiQKyEjIhWGQFINFugFCRUUPVih2N9lr11EH1v287CiviFnPjDtEn94HKntiSXea3hDtZ/7plaiCPUEgPikOF7+C0dHCc2A0yBHm3ipqdsENFsqU7fT31Fp7Isvua+WwJJS+sUMfCDs0/IynW4jEXVI/q75qjIr+x+66eARLCCLW71YPl")

private_key = """
-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEAu6y6a7moRCqHtpWjDJ7XvIRwahyh+8/cfWXlFO/KIDgrQKdI
8rWzAlKAffY+z/uwrCK5Iyx6Qm/FJ5VzcE+65dbIrdJ7bdNVtJVYp7RqzSBI0gpd
3hmZD0BlT/vzueUgclTn5F7afmMIBINgcdc40Ry7vcz1ZL6smgv2n4kCshIyIVhk
BSDRboBQkVFD1YodjfZa9dRB9b9vOwor4hZz4w7RJ/eByp7Ykl3mt4Q7Wf+6ZWog
j1BID4pDhe/gtHRwnNgNMgR5t4qanbBDRbKlO3099RaeyLL7mvlsCSUvrFDHwg7N
PyMp1uIxF1SP6u+aoyK/sfuungESwgi1u9WD5QIDAQABAoIBAQCKkxzHhDvJsXmq
CM1u+S6k5Um4IFI/BBlzgjRnhDNEHRVa1OqZRC7cbRyxZYy1t8uZHr6DSUkxGySB
eOnXKRgAs9pT9tHqoxxqjcf7dM1Tjx4V8U+kOlR5HXxxVcF+JsARi736M0uz/N2j
r3ocNOWgCk5Z9CfR7rS1vlWpMNqLrn3Hj6leCF/ueBG+xS75HBJz6qdfAsQ8wO2/
UuWivzTjBea6GistTVsHT4ER3QWBp9CUgs73fKlDpqbdeg3tOQJul7wMT4xKh2lg
TUHASNgKuEoydnk9vkXY2DQYu0wUgq768/VLrAfQmUJnsaVdg45s4adkLHkkW8Je
B8OwH/MBAoGBAOWhks+ra1fudOmaKiS3Twdu2kcLjQUffC5nBaZk5NkcHC9YnmeF
qvS1JdFC6hYfeY3GMLB6KJ+zL+XKloogrL6PQ3Rx31w7eCXEfLMMxVa/3/b0qi0l
PUJlXY5eQ2jZ9JjZZXK7SC7s4HNLQdJ5UvllaZF4jHwzDLguy6QdlLtBAoGBANE5
xN2d1983aNeziImScPbrdIjMqwawSH+d4RUUIAmjYJioZuB9/T8s6enIWJz+/ckO
1JvODlm4PVdV0wxxlDkq+jleB28J4OiW/vplBfUV3YyFojMYnWL2RhnQBcspJdFx
Wzs1mbY898u0mEf+jK4ayp78mkS2W1h8kxArUxOlAoGBAOMverd5Sidh5Utk2gMf
VNHuy4f0lp2V699gz8czFPL0C7KQA5A6P8wBGJwzjrM6iqFIjs1a3qw5tM6tI0kf
UKjrxnoDW0++Cn2bKyBeJzNPfD6xC1jE+hmhffEns5ud35AFrYfYYG8Ern+C2mlo
3T2fJtXkpWEPhKsIqTMCjS7BAoGBAJIzHuiJWoZE7sMDVh5jsQIpp6XL9ppW5mIe
jWCwTm7NtjsWOcUW5LaXiOBuudUCrY4oCdLRmt+AyYRUmYQxfZSw/mbF2MXzjjCZ
CpUnsJEA9W4zFxNDWP8E/hkdbl73YtDGuCxYmQ9p7PFwQZTVP7KNUBbmhloLXysm
6ZC75XJtAoGAKUtl6FAYUoiP1H4XKjZdTe2RzX5QRYuQpYpHLuQ7K82AD+8yHSuk
x0JN8v1olTN3a1HZmo/hb3lhZS1reyfyyZr9Eg6ezHEG5ssquXIYSGfbloA2uS5B
+HG0Uxy3O9kzbthWJnYVJDXzvserIhwgzAtgRFRdrcPj4PLBEOB0cVg=
-----END RSA PRIVATE KEY-----
"""

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "ssh_public_keys": [
      "#{public_key}"
    ],

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "ruby:2.6"
        }
      ]
    },

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "sleep 15" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    }
  }
JSON

wait_for_command_to_start("sleep 15")

File.write("/tmp/private-key", private_key)

`chmod 0600 /tmp/private-key`

system "ssh -i /tmp/private-key -o StrictHostKeyChecking=no -p 2222 root@localhost bash /tmp/ssh_jump_point ruby --version"
output = `ssh -i /tmp/private-key -o StrictHostKeyChecking=no -p 2222 root@localhost bash /tmp/ssh_jump_point ruby --version`

if output =~ /2\.6/
  puts "SSH success!"
else
  abort "Failed to set up proper SSH jump point"
  puts output
end
