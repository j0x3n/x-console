# 网页盯梢用的浏览器（B205）。官方 headless-shell 只带西文字体，
# 页面里的中文会显示成方块，所以在它上面补一套中文字体。
FROM chromedp/headless-shell:latest
# 让 GitHub 把这个镜像关联到本仓库，部署用的令牌才拉得到。
LABEL org.opencontainers.image.source=https://github.com/j0x3n/x-console
RUN apt-get update \
 && apt-get install -y --no-install-recommends fontconfig fonts-noto-cjk \
 && rm -rf /var/lib/apt/lists/* \
 && fc-cache -f
