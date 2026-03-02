FROM debian:12-slim

# 1. 设置环境变量，让某些程序直接识别上海时区
ENV TZ=Asia/Shanghai

# 2. 安装必要工具，并配置时区
RUN apt-get update && apt-get install -y \
    tzdata \
    libssl3 \
    openssl \
    iproute2 \
    openssh-server \
    cron \
    wget \
    curl \
    nano \
    vim \
    iputils-ping \
    net-tools \
    procps \
    ca-certificates \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone \
    && mkdir /var/run/sshd \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# 配置 SSH：允许 root 登录和密码认证
RUN sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config \
    && sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# 将启动脚本复制到根目录
COPY debian12/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# 暴露端口与定义持久化目录
EXPOSE 22
VOLUME ["/root", "/opt", "/usr/local/bin"]

ENTRYPOINT ["/entrypoint.sh"]
