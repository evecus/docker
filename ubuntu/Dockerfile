FROM ubuntu:24.04

# 1. 设置环境变量
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai
ENV password=root

# 2. 安装基础工具
RUN apt-get update && apt-get install -y \
    openssh-server \
    vim curl wget git net-tools iputils-ping htop ca-certificates tzdata \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# 3. 配置 SSH 服务
RUN mkdir /var/run/sshd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/UsePAM yes/UsePAM no/' /etc/ssh/sshd_config && \
    # 显式允许密码登录
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# 4. 将本地的 entrypoint.sh 复制到容器根目录并赋权
COPY ubuntu/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# 5. 暴露端口与挂载点
EXPOSE 22
VOLUME ["/root", "/opt", "/usr/local/bin"]

# 6. 设置启动入口
ENTRYPOINT ["/entrypoint.sh"]
