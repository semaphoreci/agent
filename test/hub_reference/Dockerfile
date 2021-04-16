FROM ruby:2.7

WORKDIR /app

ADD Gemfile Gemfile
ADD app.rb app.rb

RUN bundle install
CMD bundle exec ruby app.rb
