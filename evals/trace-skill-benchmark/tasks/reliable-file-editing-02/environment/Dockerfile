FROM catena/trace-skill-eval:1
USER root
COPY files/ /workspace/case/
COPY setup.sh /opt/case-setup.sh
RUN bash /opt/case-setup.sh && chown -R catena-eval:catena-eval /workspace
WORKDIR /workspace/case
ENTRYPOINT []
